package runner

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// This file implements PHP's static class members: `Class::method()`,
// `Class::$property` and the `self::` / `static::` spellings of both.
//
// A static member belongs to the class, not to an instance, so neither has a
// receiver to hang off the object model in value.go. Methods are run against a
// scope that knows only which class it is in; properties live in one bag per
// class on the Runtime (see Runtime.classStatics), which is what makes
// composer's `self::$registeredLoaders` observable from every instance and from
// a later static call.

// resolveClassName maps the contextual class names PHP allows in a static
// reference onto a real one. `self` and `static` both name the class of the
// running method (there is no inheritance, so late static binding and self are
// the same class), and `parent` has nothing to resolve to.
func resolveClassName(class string, scope *Scope) string {
	switch class {
	case "self", "static", "parent":
		if scope != nil {
			if current, ok := scope.Get("__class__"); ok {
				if name, ok := current.(string); ok && name != "" {
					return name
				}
			}
		}
	}
	return strings.TrimPrefix(class, "\\")
}

// classDefaultScope is the scope a class's declared defaults are evaluated in.
// A default is written inside the class body, so `self::X` in one has to
// resolve to that class; the scope the `new` happened to be written in knows
// nothing about it.
func (rt *Runtime) classDefaultScope(class *model.Class, caller *Scope) *Scope {
	scope := rt.newScope()
	if caller != nil {
		if file, ok := caller.Get("__FILE__"); ok {
			scope.Set("__FILE__", file)
		}
		if dir, ok := caller.Get("__DIR__"); ok {
			scope.Set("__DIR__", dir)
		}
	}
	scope.Set("__class__", class.Name)
	return scope
}

// staticBag returns the live static-property storage for a class, seeding it
// from the declared defaults on first access. The bag is keyed by the class's
// canonical name so a case-insensitive lookup and the declaration share it.
func (rt *Runtime) staticBag(class *model.Class, scope *Scope) (map[string]any, error) {
	if bag, ok := rt.classStatics[class.Name]; ok {
		return bag, nil
	}
	bag := make(map[string]any, len(class.Statics))
	defaults := rt.classDefaultScope(class, scope)
	for _, field := range class.Statics {
		var value any
		if field.Default != nil {
			evaluated, err := rt.Eval(field.Default, defaults)
			if err != nil {
				return nil, fmt.Errorf("static %s::$%s: %w", class.Name, field.Name, err)
			}
			value = evaluated
		}
		bag[field.Name] = value
	}
	rt.classStatics[class.Name] = bag
	return bag, nil
}

// staticStorage resolves a static reference to the bag holding it, autoloading
// the class if it has not been declared yet.
func (rt *Runtime) staticStorage(class string, scope *Scope) (map[string]any, error) {
	name := resolveClassName(class, scope)
	if !rt.hasClass(name) {
		if err := rt.autoload(name, scope); err != nil {
			return nil, err
		}
	}
	decl, ok := rt.lookupClass(name)
	if !ok {
		return nil, fmt.Errorf("static property %s::$...: unknown class", name)
	}
	return rt.staticBag(decl, scope)
}

// helperStaticProp reads `Class::$name`. A property that was never declared
// reads as null, matching the leniency the rest of the runtime applies to
// property and index access.
func (rt *Runtime) helperStaticProp(ref *scopeRef) func(class, name string) (any, error) {
	return func(class, name string) (any, error) {
		bag, err := rt.staticStorage(class, ref.scope)
		if err != nil {
			return nil, err
		}
		return bag[name], nil
	}
}

// setStaticProp writes `Class::$name`.
func (rt *Runtime) setStaticProp(class, name string, value any, scope *Scope) error {
	bag, err := rt.staticStorage(class, scope)
	if err != nil {
		return err
	}
	bag[name] = value
	return nil
}

// helperStaticCall dispatches `Class::method(args...)`.
//
// Three kinds of target resolve here, in the order PHP would find them:
//
//   - a host static, registered under the "Class::method" name (this is how
//     Closure::bind and friends are provided),
//   - a PHP method declared on the class, with the current `$this` forwarded
//     when an instance method calls a non-static method through `self::`,
//     `static::` or its class name,
//   - a Go constructor's package-level function, which has no static form and
//     therefore reports an undefined method.
func (rt *Runtime) helperStaticCall(ref *scopeRef) func(class, method string, args ...any) (any, error) {
	return func(class, method string, args ...any) (any, error) {
		scope := ref.scope
		name := resolveClassName(class, scope)
		// The host table is consulted by exact name only: it is one map lookup
		// on the common path, where lookupFunc's case-insensitive fallback
		// scans the whole function table on every miss, which every call to a
		// PHP static method would be.
		if fn, ok := rt.funcs[name+"::"+method]; ok {
			return rt.invokeWithScopeContext(fn, args, scope)
		}
		if !rt.hasClass(name) {
			if err := rt.autoload(name, scope); err != nil {
				return nil, err
			}
		}
		if decl, ok := rt.lookupClass(name); ok {
			fn, ok := lookupPHPMethod(decl, method)
			if !ok {
				return nil, fmt.Errorf("call to undefined method %s::%s()", name, method)
			}
			if current, ok := scope.Get("this"); !fn.Static && ok {
				if obj, ok := current.(*model.Object); ok && obj.Class == decl {
					return rt.invokeMethod(obj, fn, args, scope)
				}
			}
			return rt.invokeStatic(decl, fn, args, scope)
		}
		// No PHP class of that name: the target can only be a host static, so
		// pay for the case-insensitive lookup here rather than on every call.
		if fn, ok := rt.lookupFunc(name + "::" + method); ok {
			return rt.invokeWithScopeContext(fn, args, scope)
		}
		return nil, fmt.Errorf("static call %s::%s(): unknown class", name, method)
	}
}

// invokeStatic runs a method with no receiver. `$this` is left unset, as it is
// in a genuinely static method or a class call made without an instance, while
// `__class__` is bound so that `self::` inside the body resolves back to the
// same class. helperStaticCall forwards instance calls before reaching here.
func (rt *Runtime) invokeStatic(class *model.Class, decl *model.FuncDecl, args []any, caller *Scope) (any, error) {
	scope := rt.newScope()
	rt.pushFrame(scope)
	defer rt.popFrame()
	if decl.Filename != "" {
		setScopeFile(scope, decl.Filename)
	}
	scope.Set(argsKey, args)
	scope.Set("__class__", class.Name)
	if len(rt.observers) > 0 {
		traceScope := caller
		if traceScope == nil {
			traceScope = scope
		}
		defer rt.trace(traceScope, class.Name+"::"+decl.Name)()
	}
	if err := rt.bindParams(decl, args, scope); err != nil {
		return nil, err
	}
	value, _, runErr := rt.exec(decl.Body, scope)
	return value, combineErrors(runErr, rt.runDeferred(scope, 0))
}

// helperInvoke calls a callable held in a value: `$fn($x)`. Every PHP callable
// spelling is accepted, since Callable is what call_user_func already uses.
func (rt *Runtime) helperInvoke(ref *scopeRef) func(callee any, args ...any) (any, error) {
	return func(callee any, args ...any) (any, error) {
		scope := ref.scope
		fn, ok := rt.callableWithScope(callee, scope)
		if !ok {
			return nil, fmt.Errorf("value of type %T is not callable", callee)
		}
		return fn(args...)
	}
}
