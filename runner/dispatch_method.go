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
// Throwable method set every Go error answers. The typed fast paths it falls
// back from live in dispatch_direct.go.

// callGoMethod invokes an exported method on a Go value by reflection. It
// matches method names case-insensitively (PHP method calls are
// case-insensitive, so `$obj->get()` resolves Go's exported Get). When the
// method's first parameter is a context.Context it injects the runtime context,
// and coerces arguments to the declared parameter types.
// goMethodKey and goMethodInfo cache Go method resolution per receiver type
// and spelled name; see Runtime.goMethods.
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

// resolveGoMethod resolves the way callGoMethod always has: the exact name,
// then case-insensitively, then without underscores so a snake_case PHP
// spelling finds the idiomatic Go name (get_id -> GetID), the same ladder
// methodByNameFold climbs. It computes the bound signature and the context
// check once here, on the value, so the per-call path is one map hit and an
// index.
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

// bindGoMethod plans a method call the way reflectInvoker plans a function
// call: parameter types, variadic element and padding resolved at cache
// time, coercion through the one table per call.
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
		// A PHP position is 1-based over the arguments the script wrote, so an
		// injected context, which sits at args[0], shifts it back by one.
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

// callGoMethod invokes a Go method by reflection. It calls scopeFor only when
// the method's first parameter is a context, the one moment the runtime has to
// materialise the PHP frame; a caller with the scope in hand passes a closure
// returning it, which never escapes and costs nothing.
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
		// The class a script wrote, not the Go type carrying it: an object of a
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

// methodCallError names the method on the errors bindGoMethod reports
// nameless.
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

// throwableMethod implements PHP's Throwable interface over any Go error.
//
// stdlib registers every throwable class name to one type, so a catch clause
// binds whatever error reached it: an Exception a script threw, an error a
// binding returned, or a panic converted at the host boundary. Each answers the
// method set a script expects on a caught value. A method the concrete Go type
// defines wins, so *stdlib.Exception reports its own code.
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

// methodByNameFold finds an exported method using PHP's case-insensitive
// semantics, then retries without underscores so snake_case PHP names resolve
// idiomatic Go names (for example get_all -> GetAll).
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

// goFieldInfo is one cached field resolution: the index chain
// Value.FieldByIndexErr reads, and whether the name resolved to an exported
// field at all. structField caches a miss too, so a probe does not rescan the
// field set.
type goFieldInfo struct {
	index []int
	found bool
}

// structField resolves a property read's field once per (struct type, spelled
// name), climbing the ladder fieldByNameFold climbs: the exact name, then
// case-insensitively, then without underscores. An unexported match resolves
// as not found, which is the CanInterface check the uncached read made.
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

// fieldByNameFold finds a struct field the way methodByNameFold finds a method:
// exact match first (PHP property access on a Go struct, `$rec->value` for the
// field Value), then case-insensitively, then without underscores so snake_case
// PHP property names resolve idiomatic Go names (is_readonly -> IsReadonly).
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
