package runner

import (
	"reflect"

	"github.com/titpetric/phpscript/model"
)

// Callable resolves a PHP `callable` value into the uniform
// func(...any) (any, error) signature the runtime invokes everywhere.
//
// PHP accepts several spellings of a callable and library code written for
// stock PHP uses all of them, so `call_user_func`, `usort` and friends have to
// understand each one:
//
//   - a closure or any Go func already registered with the runtime,
//   - "function_name", naming a free function,
//   - "Class::method", naming a static method,
//   - array($object, "method"), the bound-method form,
//   - array("Class", "method"), the static form.
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
	if reflect.ValueOf(v).Kind() == reflect.Func {
		return adapt(v), true
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
	return func(args ...any) (any, error) {
		return rt.invokeMethod(obj, decl, args, scope)
	}, true
}

// staticMethod resolves Class::method without a receiver. The declaration is
// invoked against an empty instance of the class so `self::` constants still
// resolve; PHP would reject `$this` here and so does the empty receiver.
func (rt *Runtime) staticMethod(className, method string, scope *Scope) (func(...any) (any, error), bool) {
	class, ok := rt.lookupClass(className)
	if !ok {
		return nil, false
	}
	decl, ok := lookupPHPMethod(class, method)
	if !ok {
		return nil, false
	}
	return func(args ...any) (any, error) {
		return rt.invokeMethod(model.NewObject(class), decl, args, scope)
	}, true
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
