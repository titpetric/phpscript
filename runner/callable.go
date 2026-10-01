package runner

import (
	"fmt"
	"reflect"

	"github.com/titpetric/phpscript/model"
)

// Callable is a php callable as a runtime value: one declaration to run, and
// what it is bound to. It covers a closure literal, a method bound to the
// receiver it was read off, and a declared function held by name, which is
// what php's Closure covers and what a script sees it as.
//
// docs/GLOSSARY.md records the word and docs/design.md the re-entry rule it
// exists for: the value crosses to another runtime and the execution does not.
type Callable struct {
	// closure and fn are the declaration, one or the other: a literal is a
	// model.Closure, and everything a name reaches is a model.FuncDecl.
	closure *model.Closure
	fn      *model.FuncDecl

	// obj is the receiver a bound method runs against. It is unfilled for a
	// closure literal, whose `$this` arrives in env with the rest of its
	// captures, and for a global function, which has none.
	obj *model.Object

	// class stands in for obj in the Class::method spelling, which names no
	// receiver. An empty instance is built per call rather than shared, so
	// `self::` resolves and nothing crosses between two calls.
	class *model.Class

	// env is what a closure literal took from the scope it was written in.
	env closureEnv

	// name holds a declared function the program named, resolved by the runtime
	// running it rather than here. It is the one spelling whose meaning belongs
	// to the runtime rather than to the value: a name is a program counter that
	// each runtime looks up in its own function table, which is what lets a host
	// register a binding under it.
	name string

	// rt is the runtime that built the value, and call is the uniform shape on
	// it. The pair is what `on` compares against the runtime doing the calling:
	// a value is a value and crosses freely, but an execution belongs to one
	// goroutine, so a call from anywhere else has to be a call of its own.
	//
	// call is built here rather than taken as a method value where it is needed.
	// coerceArg hands it to bindings, and a method value referenced from there
	// would put the whole interpreter in the dependency graph of the package
	// variable holding the expression helpers, which the compiler reports as an
	// initialisation cycle.
	rt   *Runtime
	call func(...any) (any, error)
}

// on answers the call for the runtime doing the calling.
//
// The runtime that built the value gets the call built with it, which is every
// ordinary script-level call and costs nothing. Any other runtime gets a call of
// its own, because a Runtime is one goroutine's execution - its frames, its
// output stack, its statics, its request - and reaching into another one is not
// a race to reason about but a concurrent map write. See runner/fork.go.
//
// This is what makes a handler that wraps another callable answer on the worker
// the request arrived on: the value crossed, the execution did not.
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

// newMethod binds a method declaration to the receiver it was read off. The
// scope is the caller's, which is what an invocation's trace span is recorded
// against.
func (rt *Runtime) newMethod(obj *model.Object, decl *model.FuncDecl, scope *Scope) *Callable {
	c := &Callable{fn: decl, obj: obj, rt: rt}
	c.call = func(args ...any) (any, error) { return rt.invokeMethod(obj, decl, args, scope) }
	return c
}

// newStaticMethod binds Class::method, which has no receiver. The empty instance
// is built per call, so `self::` constants resolve and two calls share nothing;
// php would reject `$this` here and so does an instance with no properties.
func (rt *Runtime) newStaticMethod(class *model.Class, decl *model.FuncDecl, scope *Scope) *Callable {
	c := &Callable{fn: decl, class: class, rt: rt}
	c.call = func(args ...any) (any, error) {
		return rt.invokeMethod(model.NewObject(class), decl, args, scope)
	}
	return c
}

// newNamedCallable holds a declared function by name, for a host describing one
// it was handed as a string.
func (rt *Runtime) newNamedCallable(name string) *Callable {
	c := &Callable{name: name, rt: rt}
	c.call = func(args ...any) (any, error) { return rt.InvokeNamed(name, args...) }
	return c
}

// Call invokes the callable on the runtime that built it, which is what a script
// calling one means: `($this->fn)(...)` is the call `$this->fn(...)` makes.
func (c *Callable) Call(args ...any) (any, error) { return c.call(args...) }

// Invoke runs the callable on rt, in a scope holding args and nothing else -
// and, for a bound method, the receiver it was read off.
//
// It is the cross-runtime call, and it is deliberately not Call: a request is
// answered on a runtime of its own, and what crosses is the declaration rather
// than the runtime the value was built on. A name crosses as a name, so the
// runtime running it resolves it against its own function table.
func (c *Callable) Invoke(rt *Runtime, args ...any) (any, error) {
	switch {
	case c.closure != nil:
		// A statics bag per call rather than per value: two goroutines running
		// one declaration are two calls, and a `static $x` shared between them
		// would be a counter two requests were incrementing at once. php gives a
		// closure instance one bag because there a request is a process.
		env := c.env
		env.statics = map[*model.StaticVar]map[string]any{}
		return rt.invokeClosure(c.closure, args, env)
	case c.class != nil:
		// A static spelling names no receiver, so each runtime builds its own
		// empty instance and nothing crosses.
		return rt.invokeMethod(model.NewObject(c.class), c.fn, args, nil)
	case c.fn != nil:
		// The receiver is the object the method was read off, shared with every
		// other runtime answering through this callable, the way a closure's
		// captured $this is. A scope of its own, holding it and the arguments, is
		// what invokeMethod builds.
		return rt.invokeMethod(c.obj, c.fn, args, nil)
	}
	return rt.InvokeNamed(c.name, args...)
}

// Captures reports whether the callable took anything from where it was written:
// a closure's `use (...)` clause or the `$this` one written inside a method
// binds, and the receiver a bound method reads its properties off.
//
// It is the question a host asks before running one somewhere else. A callable
// that captures nothing is a function of its arguments, and a function of its
// arguments runs anywhere: a declared function held by name and a Class::method
// spelling, whose receiver is an empty instance built per call, both answer false.
func (c *Callable) Captures() bool {
	if c.closure != nil {
		return len(c.closure.Uses) > 0 || c.env.this != nil || c.env.class != nil
	}
	return c.obj != nil
}

// declaringClass answers the class a method callable belongs to, whether it came
// from a receiver or from a static spelling.
func (c *Callable) declaringClass() *model.Class {
	if c.class != nil {
		return c.class
	}
	if c.obj != nil {
		return c.obj.Class
	}
	return nil
}

// Name answers how the callable is spelled, for an error message. A closure has
// no name and says so; a method is named the way a script reads it.
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
// function by name, or Class::method. What it captured comes along and is shared
// by every runtime running it, so a callable must read what it captured and not
// write to it.
//
// The two array spellings are refused here and accepted everywhere else, which
// docs/README.md records as a decision. A Go func is refused for a different
// reason: it is not a declaration, so there is nothing for another runtime to
// run, and a host holding one already holds the call.
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
		// An object with __invoke is the same binding a bound method is, under
		// the name php reserves for it.
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

// callableReason says what a host can run somewhere else, for the refusals that
// have no reason of their own.
const callableReason = "a callable is a closure, an object with __invoke, a bound method, Class::method, or the name of a declared function"

// asStaticCallable resolves Class::method, naming which half of the spelling was
// not found: a message saying no function of that name is declared was true of
// neither.
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

// InvokeClosure runs a closure declaration on this runtime, in a scope holding
// its arguments and nothing else.
//
// It is the re-entry a host callback needs. The declaration is AST, so it runs
// on whichever runtime the caller has; what it does not get is the scope the
// closure was written in, which is why Captures has to be false for the result
// to mean anything. A fresh statics bag per call is the other half of that: two
// goroutines running the same declaration are two calls, not one function
// accumulating.
func (rt *Runtime) InvokeClosure(decl *model.Closure, args ...any) (any, error) {
	if decl == nil {
		return nil, &LookupError{Symbol: "closure", Reason: "no declaration"}
	}
	env := closureEnv{statics: map[*model.StaticVar]map[string]any{}}
	if rt.entrypoint != "" {
		env.filename = rt.entrypoint
		env.directory = rt.fileDir(rt.entrypoint)
	}
	return rt.invokeClosure(decl, args, env)
}

// Callable resolves a PHP `callable` value into the uniform
// func(...any) (any, error) signature the runtime invokes everywhere.
//
// Every spelling php accepts resolves: a *Callable, a Go func, "function_name",
// "Class::method", array($object, "method") and array("Class", "method"). It
// answers the call rather than the value, which is what a binding taking a
// callable declares; AsCallable is the other direction.
//
// The second return reports whether v was callable at all; callers turn that
// into PHP's "not a valid callback" error with their own function name.
func (rt *Runtime) Callable(v any) (func(...any) (any, error), bool) {
	return rt.callableWithScope(v, rt.newScope())
}

func (rt *Runtime) callableWithScope(v any, scope *Scope) (func(...any) (any, error), bool) {
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
	// A typed nil func is a func-shaped value that cannot be called: invoking
	// one panics inside reflect rather than reporting anything useful, so it is
	// not callable and is_callable answers so.
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
	// PHP allows array($obj, "parent::method"); only the plain form is
	// supported here, and "Class::method" in slot 1 is not valid PHP either.
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

// staticMethod resolves Class::method without a receiver, for the spellings that
// report callable-or-not rather than why.
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

// closureValue answers `greet(...)` and `$fn(...)`, php 8.1's first-class
// callable syntax over a name or a value: the callable the call site names,
// without calling it. fallback is the Call node's global-namespace name.
//
// A value that is already callable answers itself, which is what makes
// `$fn(...) === $fn` true as it is in php. Everything else resolves to a
// *Callable, so a script reads the result as php's Closure.
func (rt *Runtime) closureValue(target any, fallback string, scope *Scope) (any, error) {
	switch value := target.(type) {
	case *Callable:
		return value, nil
	case string:
		// The function table first: it holds declared functions, registered
		// bindings and the host statics registered under a "Class::method" name,
		// which is the order helperStaticCall resolves a call in.
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
		// that method rather than the object.
		return rt.closureMember(value, "__invoke", scope)
	}
	if _, ok := rt.callableWithScope(target, scope); ok {
		return target, nil
	}
	return nil, fmt.Errorf("value of type %s is not callable", phpDebugType(target))
}

// closureMember answers `$obj->method(...)` and `Class::method(...)`: the
// declaration bound to what it will run against, as a *Callable.
//
// A string target is a class name, and the declaration is bound to an empty
// instance of it so `self::` inside the body resolves - the receiver staticMethod
// builds for the same spelling. A host-backed receiver has no declaration to
// bind, so its method resolves through reflection and the bound call is the value
// itself.
func (rt *Runtime) closureMember(target any, method string, scope *Scope) (any, error) {
	switch receiver := target.(type) {
	case *model.Object:
		if receiver.Class != nil {
			if decl, ok := lookupPHPMethod(receiver.Class, method); ok {
				return rt.newMethod(receiver, decl, scope), nil
			}
		}
	case string:
		// A host static is registered under the whole spelling - DateTime::now is
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
		return rt.newMethod(model.NewObject(class), decl, scope), nil
	}
	if fn, ok := rt.boundGoCallable(target, method, scope); ok {
		return fn, nil
	}
	return nil, fmt.Errorf("call to undefined method %s::%s()", phpClassName(target), method)
}

// boundGoCallable binds an exported Go method as a callable value, the same
// resolution helperGet gives `$db->close` without parentheses. It probes before
// binding because a name no method answers is reported here rather than at the
// call, where boundGoMethod would only find out.
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
