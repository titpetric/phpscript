package runner

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// This file dispatches a method call or a property read onto a Go value by
// reflection: the name ladder PHP's case-insensitive spellings climb, the
// per-type caches that keep the ladder off the per-call path, and the
// Throwable method set every Go error answers. dispatch_direct.go holds the
// typed fast paths it falls back from.

// goMethodKey is the receiver type and spelled name goMethods caches a resolution under.
type goMethodKey struct {
	t    reflect.Type
	name string
}

type goMethodInfo struct {
	index    int
	mtype    reflect.Type
	wantsCtx bool
	found    bool
	// bind is the pre-planned call over the bound signature: coercion with
	// the parameter types resolved once, padding, Call, result reduction.
	// It takes the bound method value because the cache is per receiver
	// type, not per receiver, and the runtime because a callable argument
	// binds to the runtime making the call.
	bind func(rt *Runtime, m reflect.Value, args []any) (any, error)
	// direct is the typed call for the method shapes a handler touches per
	// request, dispatched through an interface assertion in place of
	// reflect.Value.Call and its per-call receiver, frame and result
	// allocations. Nil when no shape matches; a false from it sends the
	// call down bind with its coercion and its errors intact.
	direct func(base any, args []any) (any, bool, error)
}

// resolveGoMethod finds a method by exact, folded or underscore-free name.
func resolveGoMethod(rv reflect.Value, name string) goMethodInfo {
	t := rv.Type()
	index := -1
	if m, ok := t.MethodByName(name); ok {
		index = m.Index
	}
	if index < 0 {
		for i := 0; i < t.NumMethod(); i++ {
			if strings.EqualFold(t.Method(i).Name, name) {
				index = i
				break
			}
		}
	}
	if index < 0 {
		bare := strings.ReplaceAll(name, "_", "")
		for i := 0; i < t.NumMethod(); i++ {
			if strings.EqualFold(strings.ReplaceAll(t.Method(i).Name, "_", ""), bare) {
				index = i
				break
			}
		}
	}
	if index < 0 {
		return goMethodInfo{}
	}
	mtype := rv.Method(index).Type()
	return goMethodInfo{
		index:    index,
		mtype:    mtype,
		wantsCtx: wantsContext(mtype),
		found:    true,
		bind:     bindGoMethod(mtype),
		direct:   directGoMethod(rv.Interface(), t.Method(index).Name),
	}
}

// bindGoMethod plans a method call's coercion, variadic element and padding at cache time.
func bindGoMethod(mtype reflect.Type) func(*Runtime, reflect.Value, []any) (any, error) {
	numIn, variadic := mtype.NumIn(), mtype.IsVariadic()
	injected := injectedArgs(mtype)
	params := make([]reflect.Type, numIn)
	for i := range params {
		params[i] = mtype.In(i)
	}
	var elem reflect.Type
	if variadic {
		elem = params[numIn-1].Elem()
	}
	return func(rt *Runtime, m reflect.Value, args []any) (any, error) {
		if !variadic && len(args) > numIn {
			return nil, &ArgumentCountError{Want: numIn - injected, Got: len(args) - injected}
		}
		in := make([]reflect.Value, 0, len(args))
		// A PHP position is 1-based over the arguments the script wrote and an
		// injected context at args[0] shifts it back by one.
		offset := 1 - injected
		for i, a := range args {
			var want reflect.Type
			switch {
			case variadic && i >= numIn-1:
				want = elem
			case i < numIn:
				want = params[i]
			}
			v, ok := coerceArgOn(rt, a, want)
			if !ok {
				return nil, &TypeError{
					Position: i + offset,
					Want:     phpParamTypeName(want),
					Got:      phpDebugType(a),
				}
			}
			in = append(in, v)
		}
		for len(in) < numIn && !(variadic && len(in) >= numIn-1) {
			in = append(in, reflect.Zero(params[len(in)]))
		}
		return callResult(m.Call(in))
	}
}

// callGoMethod invokes a Go method, calling scopeFor to inject a context.
func (rt *Runtime) callGoMethod(base any, method string, args []any, scopeFor func() *Scope) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = nil
			err = recoveredError(recovered, fmt.Sprintf("%T::%s", base, method))
		}
	}()
	if base == nil {
		return nil, fmt.Errorf("call %s on nil", method)
	}
	rv := reflect.ValueOf(base)
	key := goMethodKey{t: rv.Type(), name: method}
	info, cached := rt.goMethods[key]
	if !cached {
		info = resolveGoMethod(rv, method)
		if rt.goMethods == nil {
			rt.goMethods = map[goMethodKey]goMethodInfo{}
		}
		rt.goMethods[key] = info
	}
	if !info.found {
		if value, ok := throwableMethod(base, method); ok {
			return value, nil
		}
		// The class a script wrote, not the Go type behind it: an object of a
		// declared class reports the declared name, which is the name the
		// author can look for in the source.
		return nil, fmt.Errorf("call to undefined method %s::%s()", phpClassName(base), method)
	}
	if info.wantsCtx {
		args = append([]any{rt.bindingContext(scopeFor())}, args...)
	} else if info.direct != nil {
		if result, handled, err := info.direct(base, args); handled {
			if err != nil {
				return nil, methodCallError(err, method)
			}
			return result, nil
		}
	}
	result, err = info.bind(rt, rv.Method(info.index), args)
	if err != nil {
		return nil, methodCallError(err, method)
	}
	return result, nil
}

// methodCallError names the method on the arity and type errors bindGoMethod reports.
func methodCallError(err error, method string) error {
	var arity *ArgumentCountError
	if errors.As(err, &arity) && arity.Name == "" {
		arity.Name = method
		return arity
	}
	var typeErr *TypeError
	if errors.As(err, &typeErr) && typeErr.Name == "" {
		typeErr.Name = method
		return typeErr
	}
	return err
}

// throwableMethod answers PHP's Throwable methods for any Go error, a concrete method winning.
func throwableMethod(base any, method string) (any, bool) {
	err, ok := base.(error)
	if !ok {
		return nil, false
	}
	switch strings.ToLower(strings.ReplaceAll(method, "_", "")) {
	case "getmessage", "tostring", "__tostring":
		return err.Error(), true
	case "getcode":
		return int64(0), true
	case "getprevious":
		if previous := errors.Unwrap(err); previous != nil {
			return previous, true
		}
		return nil, true
	case "getfile":
		return "", true
	case "getline":
		return int64(0), true
	case "gettrace":
		return model.NewArray(), true
	case "gettraceasstring":
		return "#0 {main}", true
	}
	return nil, false
}

// methodByNameFold finds a method by exact, folded or underscore-free name.
func methodByNameFold(rv reflect.Value, method string) reflect.Value {
	t := rv.Type()
	for i := 0; i < t.NumMethod(); i++ {
		if strings.EqualFold(t.Method(i).Name, method) {
			return rv.Method(i)
		}
	}
	method = strings.ReplaceAll(method, "_", "")
	for i := 0; i < t.NumMethod(); i++ {
		name := strings.ReplaceAll(t.Method(i).Name, "_", "")
		if strings.EqualFold(name, method) {
			return rv.Method(i)
		}
	}
	return reflect.Value{}
}

// goFieldInfo holds a field's index chain and whether Go exports it.
type goFieldInfo struct {
	index []int
	found bool
}

// structField resolves and caches a struct field, answering false for an unexported match.
func (rt *Runtime) structField(t reflect.Type, name string) ([]int, bool) {
	key := goMethodKey{t: t, name: name}
	if info, ok := rt.goFields[key]; ok {
		return info.index, info.found
	}
	field, ok := t.FieldByName(name)
	if !ok {
		field, ok = t.FieldByNameFunc(func(n string) bool { return strings.EqualFold(n, name) })
	}
	if !ok {
		folded := strings.ReplaceAll(name, "_", "")
		field, ok = t.FieldByNameFunc(func(n string) bool {
			return strings.EqualFold(strings.ReplaceAll(n, "_", ""), folded)
		})
	}
	info := goFieldInfo{index: field.Index, found: ok && field.PkgPath == ""}
	if rt.goFields == nil {
		rt.goFields = map[goMethodKey]goFieldInfo{}
	}
	rt.goFields[key] = info
	return info.index, info.found
}

// fieldByNameFold finds a struct field by exact, folded or underscore-free name.
func fieldByNameFold(value reflect.Value, name string) reflect.Value {
	if f := value.FieldByName(name); f.IsValid() {
		return f
	}
	if f := value.FieldByNameFunc(func(n string) bool { return strings.EqualFold(n, name) }); f.IsValid() {
		return f
	}
	folded := strings.ReplaceAll(name, "_", "")
	return value.FieldByNameFunc(func(n string) bool {
		return strings.EqualFold(strings.ReplaceAll(n, "_", ""), folded)
	})
}
