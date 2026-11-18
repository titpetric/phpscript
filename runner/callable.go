package runner

import (
	"fmt"
	"reflect"

	"github.com/titpetric/phpscript/model"
)

// Callable is a php callable as a runtime value: one declaration to run, and
// whatever it binds to. The type covers a closure literal, a method bound to
// its receiver, or a declared function held by name. php's Closure covers the
// same three. A script cannot tell them apart.
//
// docs/GLOSSARY.md records the word and docs/design.md the re-entry rule it
// exists for: the value crosses to another runtime and the execution does not.
type Callable struct {
	// closure and fn hold the declaration, one of the two: a literal is a
	// model.Closure. A name reaches a model.FuncDecl.
	closure *model.Closure
	fn      *model.FuncDecl

	// obj is the receiver a bound method runs on. It stays empty for a
	// closure literal, whose `$this` arrives in env with the other captures,
	// and for a global function, which names no receiver.
	obj *model.Object

	// class holds the class for the Class::method spelling, which names no
	// receiver. Each call builds a separate empty instance, `self::` resolves
	// and nothing crosses between two calls.
	class *model.Class

	// env holds a closure literal's captures from its defining scope.
	env closureEnv

	// name holds a declared function the program named. The runtime running it
	// resolves the name; this value does not. This spelling alone holds
	// meaning that belongs to the runtime, not to the value: a name is a
	// program counter that each runtime looks up in a separate function table.
	// A host can register a binding under that name.
	name string

	// rt is the runtime that built the value; call is the uniform shape on it.
	// `on` compares that pair against the runtime doing the calling: a value is
	// a value and crosses freely. An execution belongs to one goroutine. A call
	// from anywhere else lands in Invoke.
	//
	// This constructor fills call. No caller takes it as a method value at the
	// point of use. coerceArg passes it to bindings; a method value referenced
	// from there would put the whole interpreter in the dependency graph of the
	// package variable holding the expression helpers. The compiler reports
	// that as an initialisation cycle.
	rt   *Runtime
	call func(...any) (any, error)
}

// on answers the call for the calling runtime, wrapping Invoke for another.
func (c *Callable) on(rt *Runtime) func(...any) (any, error) {
	if rt == nil || rt == c.rt {
		return c.call
	}
	return func(args ...any) (any, error) { return c.Invoke(rt, args...) }
}

// newClosure materialises a closure literal as a value.
func (rt *Runtime) newClosure(decl *model.Closure, env closureEnv) *Callable {
	c := &Callable{closure: decl, env: env, rt: rt}
	c.call = func(args ...any) (any, error) { return rt.invokeClosure(decl, args, env) }
	return c
}

// newMethod binds a method declaration to its receiver, with the caller's scope.
func (rt *Runtime) newMethod(obj *model.Object, decl *model.FuncDecl, scope *Scope) *Callable {
	c := &Callable{fn: decl, obj: obj, rt: rt}
	c.call = func(args ...any) (any, error) { return rt.invokeMethod(obj, decl, args, scope) }
	return c
}

// newStaticMethod binds Class::method, building a fresh empty instance for each call.
func (rt *Runtime) newStaticMethod(class *model.Class, decl *model.FuncDecl, scope *Scope) *Callable {
	c := &Callable{fn: decl, class: class, rt: rt}
	c.call = func(args ...any) (any, error) {
		return rt.invokeMethod(model.NewObject(class), decl, args, scope)
	}
	return c
}

// newNamedCallable holds a declared function by the name a host passes.
func (rt *Runtime) newNamedCallable(name string) *Callable {
	c := &Callable{name: name, rt: rt}
	c.call = func(args ...any) (any, error) { return rt.InvokeNamed(name, args...) }
	return c
}

// Call invokes the callable on the runtime that built it.
func (c *Callable) Call(args ...any) (any, error) { return c.call(args...) }

// Invoke runs the callable on rt, in a scope holding args alone - and, for a
// bound method, its receiver. The declaration crosses while the runtime that
// built the value stays behind; a name resolves against rt's function table.
func (c *Callable) Invoke(rt *Runtime, args ...any) (any, error) {
	switch {
	case c.closure != nil:
		// A statics bag per call, not per value: two goroutines running one
		// declaration are two calls. A `static $x` shared between the two
		// would be a counter two requests increment at once. php keeps one
		// bag per closure instance because there a request is a process.
		env := c.env
		env.statics = map[*model.StaticVar]map[string]any{}
		return rt.invokeClosure(c.closure, args, env)
	case c.class != nil:
		// A static spelling names no receiver. Each runtime builds a separate
		// empty instance; nothing crosses.
		return rt.invokeMethod(model.NewObject(c.class), c.fn, args, nil)
	case c.fn != nil:
		// The receiver is the object the method hangs off. Every runtime
		// answering through this callable shares it, as a closure's captured
		// $this does. invokeMethod builds a fresh scope holding
		// the receiver and the arguments.
		return rt.invokeMethod(c.obj, c.fn, args, nil)
	}
	return rt.InvokeNamed(c.name, args...)
}

// Captures reports whether the callable holds anything from its defining
// scope: a `use (...)` clause, a `$this`, a receiver.
func (c *Callable) Captures() bool {
	if c.closure != nil {
		return len(c.closure.Uses) > 0 || c.env.this != nil || c.env.class != nil
	}
	return c.obj != nil
}

// declaringClass answers a method callable's class, from a receiver or static spelling.
func (c *Callable) declaringClass() *model.Class {
	if c.class != nil {
		return c.class
	}
	if c.obj != nil {
		return c.obj.Class
	}
	return nil
}

// Name answers the spelling of the callable, for an error message. A closure
// answers "closure"; a method answers the way a script reads it.
func (c *Callable) Name() string {
	switch {
	case c.closure != nil:
		return "closure"
	case c.fn != nil:
		if class := c.declaringClass(); class != nil {
			return class.Name + "::" + c.fn.Name
		}
		return c.fn.Name
	}
	return c.name
}

// AsCallable describes v as something a host can run on another runtime: a
// closure, a method bound to its receiver, an object with __invoke, a declared
// function by name, or Class::method. The captures come along. Every runtime
// running the value shares those captures; a callable must read them, never
// write them.
//
// This constructor refuses the two array spellings that everything else
// accepts, which docs/README.md records as a decision. A Go func fails for a
// different reason: it holds no declaration, another runtime has nothing
// to run. A host holding one already holds the call.
func (rt *Runtime) AsCallable(v any) (*Callable, error) {
	switch value := v.(type) {
	case string:
		if value == "" {
			return nil, &LookupError{Symbol: value, Reason: "the handler name is empty"}
		}
		if class, method, ok := splitStaticCallable(value); ok {
			return rt.asStaticCallable(value, class, method)
		}
		if !rt.FunctionExists(value) {
			return nil, &LookupError{Symbol: value, Reason: "no PHP function of that name is declared"}
		}
		return rt.newNamedCallable(value), nil
	case *Callable:
		return value, nil
	case *model.Object:
		// An object with __invoke binds like a bound method, with the name php
		// reserves for it.
		if value.Class != nil {
			if decl, ok := lookupPHPMethod(value.Class, "__invoke"); ok {
				return rt.newMethod(value, decl, nil), nil
			}
		}
		return nil, &LookupError{Symbol: phpClassName(value), Reason: callableReason}
	case *model.Array:
		return nil, &LookupError{
			Symbol: "array",
			Reason: "the array($object, \"method\") spelling is not a handler here; read the method off its receiver instead, as $this->method",
		}
	}
	return nil, &LookupError{Symbol: phpDebugType(v), Reason: callableReason}
}

// callableReason names the shapes a host can run elsewhere, for a bare refusal.
const callableReason = "a callable is a closure, an object with __invoke, a bound method, Class::method, or the name of a declared function"

// asStaticCallable resolves Class::method, naming the class or the method it cannot find.
func (rt *Runtime) asStaticCallable(spelled, className, method string) (*Callable, error) {
	class, ok := rt.lookupClass(className)
	if !ok {
		return nil, &LookupError{Symbol: spelled, Reason: "no class of that name is declared"}
	}
	decl, ok := lookupPHPMethod(class, method)
	if !ok {
		return nil, &LookupError{Symbol: spelled, Reason: "the class declares no method of that name"}
	}
	return rt.newStaticMethod(class, decl, nil), nil
}

// Callable resolves a PHP callable into `func(...any) (any, error)`.
func (rt *Runtime) Callable(v any) (func(...any) (any, error), bool) {
	return rt.callableWithScope(rt.newScope(), v)
}

func (rt *Runtime) callableWithScope(scope *Scope, v any) (func(...any) (any, error), bool) {
	switch value := v.(type) {
	case nil:
		return nil, false
	case func(...any) (any, error):
		return value, true
	case *Callable:
		return value.on(rt), true
	case string:
		return rt.callableFromString(value, scope)
	case *model.Array:
		return rt.callableFromArray(value, scope)
	case *model.Object:
		// An object with __invoke is callable in PHP; the same lookup also
		// covers host objects exposing the method under either casing.
		if fn, ok := rt.boundMethod(value, "__invoke", scope); ok {
			return fn, true
		}
		return nil, false
	}
	// A typed nil func is a func-shaped value nothing can call: invoking one
	// panics inside reflect and reports nothing useful and it counts as
	// uncallable and is_callable answers false.
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Func && !rv.IsNil() {
		return adaptOn(rt, v), true
	}
	return nil, false
}

// callableFromString resolves "function" and "Class::method" spellings.
func (rt *Runtime) callableFromString(name string, scope *Scope) (func(...any) (any, error), bool) {
	if class, method, ok := splitStaticCallable(name); ok {
		return rt.staticMethod(class, method, scope)
	}
	fn, ok := rt.lookupFunc(name)
	if !ok {
		return nil, false
	}
	return func(args ...any) (any, error) {
		return rt.invokeWithScopeContext(fn, args, scope)
	}, true
}

// callableFromArray resolves array($object, "method") and array("Class", "method").
func (rt *Runtime) callableFromArray(arr *model.Array, scope *Scope) (func(...any) (any, error), bool) {
	if arr.Len() != 2 {
		return nil, false
	}
	target, okTarget := arr.Get(int64(0))
	name, okName := arr.Get(int64(1))
	if !okTarget || !okName {
		return nil, false
	}
	method, ok := name.(string)
	if !ok {
		return nil, false
	}
	// PHP allows array($obj, "parent::method"); this resolver takes the plain
	// form alone, and "Class::method" in slot 1 is not valid PHP either.
	switch subject := target.(type) {
	case *model.Object:
		return rt.boundMethod(subject, method, scope)
	case string:
		return rt.staticMethod(subject, method, scope)
	}
	// A Go-backed object (a host class instance) exposes its methods through
	// reflection, the same path `$obj->method` takes.
	if rv := reflect.ValueOf(target); rv.IsValid() {
		if m := rv.MethodByName(method); m.IsValid() {
			return rt.boundGoMethod(target, method, scope), true
		}
		if m := methodByNameFold(rv, method); m.IsValid() {
			return rt.boundGoMethod(target, method, scope), true
		}
	}
	return nil, false
}

// boundMethod binds a PHP method declaration to its receiver.
func (rt *Runtime) boundMethod(obj *model.Object, method string, scope *Scope) (func(...any) (any, error), bool) {
	if obj.Class == nil {
		return nil, false
	}
	decl, ok := lookupPHPMethod(obj.Class, method)
	if !ok {
		return nil, false
	}
	return rt.newMethod(obj, decl, scope).on(rt), true
}

// staticMethod resolves Class::method without a receiver, reporting callable-or-not only.
func (rt *Runtime) staticMethod(className, method string, scope *Scope) (func(...any) (any, error), bool) {
	class, ok := rt.lookupClass(className)
	if !ok {
		return nil, false
	}
	decl, ok := lookupPHPMethod(class, method)
	if !ok {
		return nil, false
	}
	return rt.newStaticMethod(class, decl, scope).on(rt), true
}

// splitStaticCallable splits "Class::method" and reports whether it matched.
func splitStaticCallable(name string) (string, string, bool) {
	for i := 0; i+1 < len(name); i++ {
		if name[i] == ':' && name[i+1] == ':' {
			return name[:i], name[i+2:], true
		}
	}
	return "", "", false
}

// closureValue answers first-class callable syntax, returning the callable without calling it.
func (rt *Runtime) closureValue(target any, fallback string, scope *Scope) (any, error) {
	switch value := target.(type) {
	case *Callable:
		return value, nil
	case string:
		// The function table first: it holds declared functions, registered
		// bindings and the host statics registered with a "Class::method" name,
		// the order helperStaticCall resolves a call in.
		for _, spelled := range [2]string{value, fallback} {
			if spelled == "" {
				continue
			}
			if _, ok := rt.lookupEntry(spelled); ok {
				return rt.newNamedCallable(spelled), nil
			}
		}
		if class, method, ok := splitStaticCallable(value); ok {
			return rt.closureMember(class, method, scope)
		}
		return nil, fmt.Errorf("call to undefined function %s()", value)
	case *model.Object:
		// An object with __invoke is callable, and php answers a Closure over
		// that method and not the object itself.
		return rt.closureMember(value, "__invoke", scope)
	}
	if _, ok := rt.callableWithScope(scope, target); ok {
		return target, nil
	}
	return nil, fmt.Errorf("value of type %s is not callable", phpDebugType(target))
}

// closureMember answers $obj->method(...) and Class::method(...) as a value.
func (rt *Runtime) closureMember(target any, method string, scope *Scope) (any, error) {
	switch receiver := target.(type) {
	case *model.Object:
		if receiver.Class != nil {
			if decl, ok := lookupPHPMethod(receiver.Class, method); ok {
				return rt.newMethod(receiver, decl, scope), nil
			}
		}
	case string:
		// A host registers a static with the whole spelling - DateTime::now is
		// a function, not a class with methods - so the table answers first.
		if _, ok := rt.lookupEntry(receiver + "::" + method); ok {
			return rt.newNamedCallable(receiver + "::" + method), nil
		}
		if !rt.hasClass(receiver) {
			if err := rt.autoload(receiver, scope); err != nil {
				return nil, err
			}
		}
		class, ok := rt.lookupClass(receiver)
		if !ok {
			return nil, fmt.Errorf("class %q not found", receiver)
		}
		decl, ok := lookupPHPMethod(class, method)
		if !ok {
			return nil, fmt.Errorf("call to undefined method %s::%s()", receiver, method)
		}
		return rt.newStaticMethod(class, decl, scope), nil
	}
	if fn, ok := rt.boundGoCallable(target, method, scope); ok {
		return fn, nil
	}
	return nil, fmt.Errorf("call to undefined method %s::%s()", phpClassName(target), method)
}

// boundGoCallable binds an exported Go method as a callable, probing first.
func (rt *Runtime) boundGoCallable(target any, method string, scope *Scope) (func(...any) (any, error), bool) {
	rv := reflect.ValueOf(target)
	if !rv.IsValid() {
		return nil, false
	}
	if !rv.MethodByName(method).IsValid() && !methodByNameFold(rv, method).IsValid() {
		return nil, false
	}
	return rt.boundGoMethod(target, method, scope), true
}
