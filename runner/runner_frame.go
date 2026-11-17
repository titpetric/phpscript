package runner

import (
	"errors"

	"github.com/titpetric/phpscript/model"
)

// This file runs a PHP callee in its own frame: a named function, a
// method with $this bound, a closure over its captured environment, with the
// parameter binding they share and the deferred and shutdown callbacks that
// unwind a frame and a program.

// invokeFunc runs a user-defined function in a fresh scope.
func (rt *Runtime) invokeFunc(decl *model.FuncDecl, args []any) (any, error) {
	scope := rt.acquireScope()
	defer rt.releaseScope(scope)
	rt.pushFrame(scope)
	defer rt.popFrame()
	if decl.Filename != "" {
		rt.setScopeFile(scope, decl.Filename)
	}
	scope.args = args
	if err := rt.bindParams(decl, args, scope); err != nil {
		return nil, err
	}
	val, _, runErr := rt.exec(decl.Body, scope)
	return val, combineErrors(runErr, rt.runDeferred(scope, 0))
}

// invokeMethod runs a method with $this bound to obj in a fresh scope.
func (rt *Runtime) invokeMethod(obj *model.Object, decl *model.FuncDecl, args []any, caller *Scope) (any, error) {
	scope := rt.acquireScope()
	defer rt.releaseScope(scope)
	rt.pushFrame(scope)
	defer rt.popFrame()
	if decl.Filename != "" {
		rt.setScopeFile(scope, decl.Filename)
	}
	scope.Set("this", obj)
	scope.args = args
	if obj.Class != nil {
		scope.Set("__class__", rt.boxed(obj.Class.Name))
	}
	// The span name costs two string builds and they happen only when an
	// observer is listening.
	if len(rt.observers) > 0 {
		name := obj.ID
		if name == "" && obj.Class != nil {
			name = obj.Class.Name
		}
		if name != "" {
			name += "."
		}
		traceScope := caller
		if traceScope == nil {
			traceScope = scope
		}
		defer rt.trace(traceScope, name+decl.Name)()
	}
	if err := rt.bindParams(decl, args, scope); err != nil {
		return nil, err
	}
	val, _, runErr := rt.exec(decl.Body, scope)
	return val, combineErrors(runErr, rt.runDeferred(scope, 0))
}

// closureEnv holds what a closure captures from its defining scope, snapshotted once at creation.
type closureEnv struct {
	filename  any
	directory any
	captured  map[string]any
	this      any
	class     any

	// statics holds this closure value's function-static bags: PHP keeps a
	// `static $x` storage per closure instance and two counters built by
	// the same factory count independently. Keyed like Runtime.funcStatics.
	statics map[*model.StaticVar]map[string]any
}

// captureClosureEnv snapshots scope for a closure declared in it.
func captureClosureEnv(cl *model.Closure, scope *Scope) closureEnv {
	env := closureEnv{statics: map[*model.StaticVar]map[string]any{}}
	env.filename, _ = scope.Get("__FILE__")
	env.directory, _ = scope.Get("__DIR__")
	if !cl.Static {
		env.this, _ = scope.Get("this")
		env.class, _ = scope.Get("__class__")
	}
	if len(cl.Uses) > 0 {
		env.captured = make(map[string]any, len(cl.Uses))
		for _, use := range cl.Uses {
			// A by-reference capture binds the same value a by-value one does:
			// the runtime has no reference cells and the distinction only shows
			// up in writes the closure makes back to the enclosing frame, which
			// it cannot do either way.
			value, _ := scope.Get(use.Name)
			env.captured[use.Name] = value
		}
	}
	return env
}

// invokeClosure runs a closure in a fresh scope seeded with its captured environment alone.
func (rt *Runtime) invokeClosure(cl *model.Closure, args []any, env closureEnv) (any, error) {
	scope := rt.newScope()
	rt.pushFrame(scope)
	defer rt.popFrame()
	scope.args = args
	if env.filename != nil {
		scope.Set("__FILE__", env.filename)
	}
	if env.directory != nil {
		scope.Set("__DIR__", env.directory)
	}
	if env.this != nil {
		scope.Set("this", env.this)
	}
	if env.class != nil {
		scope.Set("__class__", env.class)
	}
	for name, value := range env.captured {
		scope.Set(name, value)
	}
	scope.Set(staticsKey, env.statics)
	decl := &model.FuncDecl{Params: cl.Params, Body: cl.Body}
	if err := rt.bindParams(decl, args, scope); err != nil {
		return nil, err
	}
	val, _, runErr := rt.exec(cl.Body, scope)
	return val, combineErrors(runErr, rt.runDeferred(scope, 0))
}

// runDeferred invokes callbacks registered since mark in last-in, first-out order, each once.
func (rt *Runtime) runDeferred(scope *Scope, mark int) error {
	var errs []error
	for len(scope.deferred) > mark {
		i := len(scope.deferred) - 1
		callback := scope.deferred[i]
		scope.deferred[i] = nil
		scope.deferred = scope.deferred[:i]
		if _, err := rt.invokeWithScopeContext(callback, nil, scope); err != nil {
			errs = append(errs, err)
		}
	}
	return combineErrors(errs...)
}

func (rt *Runtime) runShutdown() error {
	if len(rt.shutdown) == 0 {
		return nil
	}
	defer rt.suspendDeadline()()

	var errs []error
	scope := rt.newScope()
	rt.pushFrame(scope)
	defer rt.popFrame()
	for len(rt.shutdown) > 0 {
		callback := rt.shutdown[0]
		rt.shutdown[0] = nil
		rt.shutdown = rt.shutdown[1:]
		if _, err := rt.invokeWithScopeContext(callback, nil, scope); err != nil {
			errs = append(errs, err)
		}
	}
	return combineErrors(errs...)
}

// combineErrors returns a single error unchanged and joins more than one.
func combineErrors(errs ...error) error {
	nonNil := errs[:0]
	for _, err := range errs {
		if err != nil {
			nonNil = append(nonNil, err)
		}
	}
	switch len(nonNil) {
	case 0:
		return nil
	case 1:
		return nonNil[0]
	default:
		return errors.Join(nonNil...)
	}
}

// bindParams binds positional args to parameters, applying defaults and collecting a variadic tail.
func (rt *Runtime) bindParams(decl *model.FuncDecl, args []any, scope *Scope) error {
	for i, p := range decl.Params {
		if p.Variadic {
			// min, because a caller that stopped short of the variadic
			// parameter leaves i past the end: `f($a, $b, ...$rest)` called as
			// `f("x")` reaches here with i at 2 and one argument, and args[i:]
			// panics on the slice bound.
			tail := args[min(i, len(args)):]
			rest := model.NewArraySize(len(tail))
			for _, arg := range tail {
				rest.Append(arg)
			}
			scope.Set(p.Name, rest)
			return nil
		}
		if i < len(args) {
			scope.Set(p.Name, args[i])
			continue
		}
		if p.Default != nil {
			v, err := rt.Eval(p.Default, scope)
			if err != nil {
				return err
			}
			scope.Set(p.Name, v)
			continue
		}
		scope.Set(p.Name, nil)
	}
	return nil
}
