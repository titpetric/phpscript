package runner

import (
	"github.com/titpetric/phpscript/model"
)

// Closure is a PHP anonymous function as a runtime value.
//
// It is callable like any other host callable, and it additionally answers what
// it is made of, which is what lets a host run one somewhere else. A closure is
// a declaration plus the scope it was written in; the declaration is AST and
// goes anywhere, the scope belongs to the runtime that built it.
//
// The value used to be a bare func(...any) (any, error) closing over both. That
// shape is still what everything invokes - coerceArg hands Call to any binding
// declaring it, and Callable answers with it - but it left the declaration
// unreachable, so a callback could only ever run where it was written.
type Closure struct {
	rt   *Runtime
	decl *model.Closure
	env  closureEnv

	// call is the uniform shape, built here rather than taken as a method value
	// where it is needed. coerceArg hands it to bindings, and a method value
	// referenced from there would put the whole interpreter in the dependency
	// graph of the package variable holding the expression helpers, which the
	// compiler reports as an initialisation cycle.
	call func(...any) (any, error)
}

// newClosure materialises a closure literal as a value.
func (rt *Runtime) newClosure(decl *model.Closure, env closureEnv) *Closure {
	c := &Closure{rt: rt, decl: decl, env: env}
	c.call = func(args ...any) (any, error) { return rt.invokeClosure(decl, args, env) }
	return c
}

// Call invokes the closure where it was written, which is what a script calling
// one means.
func (c *Closure) Call(args ...any) (any, error) { return c.call(args...) }

// uniform answers the shape every binding that takes a callable declares.
func (c *Closure) uniform() func(...any) (any, error) { return c.call }

// Declaration answers the closure's own AST, which is a declaration and nothing
// else: parameters and a body, shared and never written to.
func (c *Closure) Declaration() *model.Closure { return c.decl }

// Captures reports whether the closure took anything from the scope it was
// written in: a `use (...)` clause, or the `$this` a closure written inside a
// method binds.
//
// It is the question a host asks before running one somewhere else. A closure
// that captures nothing is a function of its arguments, and a function of its
// arguments runs anywhere.
func (c *Closure) Captures() bool {
	return len(c.decl.Uses) > 0 || c.env.this != nil || c.env.class != nil
}

// invokeCallbackClosure runs a callback's declaration on this runtime with what
// it captured, and a statics bag of its own.
//
// The bag is per call rather than per closure value: two goroutines running one
// declaration are two calls, and a `static $x` shared between them would be a
// counter two requests were incrementing at once. php gives a closure instance
// one bag because there a request is a process.
func (rt *Runtime) invokeCallbackClosure(decl *model.Closure, env closureEnv, args []any) (any, error) {
	if decl == nil {
		return nil, &LookupError{Symbol: "closure", Reason: "no declaration"}
	}
	env.statics = map[*model.StaticVar]map[string]any{}
	return rt.invokeClosure(decl, args, env)
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

// AsCallback describes v as something a host can run on another runtime: a
// closure, or a declared function by name.
//
// A closure brings what it captured. `use (...)` values and the $this a closure
// written inside a method binds come along as they are, which means every
// runtime running it shares them. That is what a handler closing over its
// configuration wants and it is how a Go handler closing over a struct behaves;
// it is also why a callback must not write to what it captured. Reading is
// fine, and a handler's own state arrives in its arguments.
//
// The `array($object, "method")` spelling of a callable is not accepted. It is
// a callable everywhere else - call_user_func and usort take it - but not a
// handler: a handler is a closure, including one held in a property, and
// docs/README.md records that.
func (rt *Runtime) AsCallback(v any) (Callback, error) {
	switch value := v.(type) {
	case string:
		name := value
		if name == "" {
			return Callback{}, &LookupError{Symbol: name, Reason: "the handler name is empty"}
		}
		if !rt.FunctionExists(name) {
			return Callback{}, &LookupError{Symbol: name, Reason: "no PHP function of that name is declared"}
		}
		return Callback{name: name}, nil
	case *Closure:
		return Callback{closure: value.decl, env: value.env}, nil
	}
	return Callback{}, &LookupError{
		Symbol: "callback",
		Reason: "a callback is a closure, or the name of a declared function",
	}
}

// SetPreparer records how a host installs this runtime's bindings, so that Fork
// can install the same ones on a child.
//
// stdlib.Register calls it. A host registering its own bindings on top passes a
// function that installs those too, or its forks will not have them.
func (rt *Runtime) SetPreparer(prepare func(*Runtime)) { rt.prepare = prepare }

// Callback is a PHP function a host can run on any runtime that has its
// declarations: a name, or a closure declaration.
//
// Both are program counters. Everything the call needs arrives in its
// arguments, which is what makes one safe to run on a goroutine of its own.
type Callback struct {
	name    string
	closure *model.Closure
	env     closureEnv
}

// Invoke runs the callback on rt, in a scope holding args and nothing else.
func (c Callback) Invoke(rt *Runtime, args ...any) (any, error) {
	if c.closure != nil {
		return rt.invokeCallbackClosure(c.closure, c.env, args)
	}
	return rt.InvokeNamed(c.name, args...)
}

// Name answers how the callback is spelled, for an error message. A closure has
// no name and says so.
func (c Callback) Name() string {
	if c.closure != nil {
		return "closure"
	}
	return c.name
}
