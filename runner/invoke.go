package runner

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/titpetric/phpscript/model"
)

// This file is the host-call boundary: how PHP calls a Go callable. It holds
// the context-injection check, the panic recovery a binding runs
// under, the coercion of a script's arguments to the declared parameter
// types, and the reduction of a reflect Call result to the value a script
// sees. dispatch.go holds the pre-bound dispatch it feeds.

// contextType is context.Context as a reflect.Type for first-argument injection.
var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()

var errorType = reflect.TypeOf((*error)(nil)).Elem()

var durationType = reflect.TypeOf(time.Duration(0))

// callbackPanic sends a PHP callback's error out through a Go frame with no error return.
type callbackPanic struct{ err error }

// recoveredError turns a recovered value into the error a script sees, unwrapping callbackPanic.
func recoveredError(recovered any, callable string) error {
	if carried, ok := recovered.(callbackPanic); ok {
		return carried.err
	}
	return &HostPanicError{Callable: callable, Value: recovered}
}

// wantsContext reports whether fn's first parameter is a context.Context.
func wantsContext(t reflect.Type) bool {
	return t.Kind() == reflect.Func && t.NumIn() > 0 && t.In(0) == contextType
}

// injectedArgs counts the context prepended to a script's arguments, one or none.
func injectedArgs(t reflect.Type) int {
	if wantsContext(t) {
		return 1
	}
	return 0
}

// invokeWithScopeContext prepends the active frame's context, then invokes fn.
func (rt *Runtime) invokeWithScopeContext(fn any, args []any, scope *Scope) (any, error) {
	if wantsContext(reflect.TypeOf(fn)) {
		full := make([]any, 0, len(args)+1)
		full = append(full, rt.bindingContext(scope))
		full = append(full, args...)
		args = full
	}
	result, err := invokeAny(rt, fn, args)
	if err == nil && rt.opts.MemoryLimit > 0 {
		// Burst guard: a single host call can take far more memory than the
		// per-statement checkpoint interval sees (str_repeat, file reads).
		// The shallow estimate only decides when to walk early; the walk is
		// the truth and resets the pending counter.
		if rt.memPending += EstimateValueSize(result); rt.memPending > rt.opts.MemoryLimit.Bytes()/8 {
			if memErr := rt.checkMemory(); memErr != nil {
				return nil, memErr
			}
		}
	}
	return result, err
}

// adapt wraps a Go callable in the uniform func(...any) (any, error) shape.
func adapt(fn any) func(...any) (any, error) {
	inv := newInvoker(fn)
	return func(args ...any) (any, error) { return inv.call(nil, args) }
}

// adaptOn adapts fn bound to rt; a callable parameter calls where the call sits.
func adaptOn(rt *Runtime, fn any) func(...any) (any, error) {
	inv := newInvoker(fn)
	return func(args ...any) (any, error) { return inv.call(rt, args) }
}

// nameCallError fills the PHP name into a call error the Go signature alone lacks.
func nameCallError(err error, name string) error {
	// The nil check keeps the allocation count down: the errors.As targets
	// below take their address and escape, which costs a heap allocation on
	// every call, including the ones that succeeded.
	if err == nil {
		return nil
	}
	var count *ArgumentCountError
	if errors.As(err, &count) && count.Name == "" {
		count.Name = name
		return err
	}
	var mismatch *TypeError
	if errors.As(err, &mismatch) && mismatch.Name == "" {
		mismatch.Name = name
	}
	return err
}

// phpParamTypeName spells a declared parameter type as a PHP TypeError does.
func phpParamTypeName(t reflect.Type) string {
	if t == nil {
		return "mixed"
	}
	if t == reflect.TypeOf((*model.Array)(nil)) {
		return "array"
	}
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "int"
	case reflect.Float32, reflect.Float64:
		return "float"
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "bool"
	case reflect.Slice, reflect.Map, reflect.Array:
		return "array"
	case reflect.Func:
		return "callable"
	case reflect.Interface:
		return "mixed"
	}
	return t.String()
}

// phpDebugType names a value the way get_debug_type does.
func phpDebugType(v any) string {
	switch value := v.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case string, []byte:
		return "string"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "int"
	case float32, float64:
		return "float"
	case *model.Array:
		return "array"
	case *model.Object:
		if value.Class != nil {
			return value.Class.Name
		}
		return "object"
	case *Callable:
		// get_debug_type() names it Closure, the class php answers for a
		// first-class callable.
		return "Closure"
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array:
		return "array"
	case reflect.Func:
		return "Closure"
	}
	return rv.Type().String()
}

// plural renders a count with its noun, as PHP spells an argument count error.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// argAt returns argument i, or nil when the script passed fewer.
func argAt(args []any, i int) any {
	if i < len(args) {
		return args[i]
	}
	return nil
}

// argsTail2 returns the variadic tail after two fixed parameters, aliased like argsTail.
func argsTail2(args []any) []any {
	if len(args) > 2 {
		return args[2:]
	}
	return nil
}

// argsTail returns the tail after the first argument, aliasing the slice without copying.
func argsTail(args []any) []any {
	if len(args) > 1 {
		return args[1:]
	}
	return nil
}

// invokeAny calls fn with args, coercing them to the declared parameter types.
func invokeAny(rt *Runtime, fn any, args []any) (result any, err error) {
	// The uniform ABI is the hot entry-less shape: a compiled closure, a bound
	// method, a Callable resolution, an adapt output. One guard, no construction.
	// A callable value resolves against rt, the runtime doing the calling.
	if c, ok := fn.(*Callable); ok {
		fn = c.on(rt)
	}
	if f, ok := fn.(func(...any) (any, error)); ok {
		defer func() {
			if recovered := recover(); recovered != nil {
				result = nil
				err = recoveredError(recovered, fmt.Sprintf("%T", fn))
			}
		}()
		return f(args...)
	}
	// Everything registered dispatches through a cached entry invoker; a
	// callable that never had a registration point builds its invoker here,
	// per call, which is the cost invokeAny paid on every call.
	return newInvoker(fn).call(rt, args)
}

// coerceArgOn coerces v for a call on rt, binding a callable to rt.
func coerceArgOn(rt *Runtime, v any, want reflect.Type) (reflect.Value, bool) {
	if want == nil || want.Kind() != reflect.Func {
		return coerceArg(want, v)
	}
	if rv := reflect.ValueOf(v); rv.IsValid() && rv.Type().AssignableTo(want) {
		return rv, true
	}
	if c, ok := v.(*Callable); ok {
		if uniform := reflect.ValueOf(c.on(rt)); uniform.Type().AssignableTo(want) {
			return uniform, true
		}
	}
	// A binding may declare its callback in its own terms in place of the
	// uniform shape, as regexp.Regexp.ReplaceAllStringFunc declares
	// func(string) string. Every spelling PHP calls a callable reaches it,
	// because rt.Callable answers them all: a closure on either engine, a
	// declared function by name, Class::method, array($obj, "method").
	if call, ok := rt.Callable(v); ok {
		// The uniform shape is variadic, which adaptCallable refuses, and needs
		// no wrapper anyway: Callable already answers that shape. Reaching it
		// through the resolution, and not through the *Callable case above, is
		// what lets a name, a Class::method and the two array spellings fill a
		// binding that declares one.
		if uniform := reflect.ValueOf(call); uniform.Type().AssignableTo(want) {
			return uniform, true
		}
		if fn, ok := adaptCallable(call, want); ok {
			return fn, true
		}
	}
	return coerceArg(want, v)
}

// adaptCallable wraps a PHP callable in the Go function type a binding declared.
func adaptCallable(call func(...any) (any, error), want reflect.Type) (reflect.Value, bool) {
	if want.IsVariadic() || want.NumOut() > 1 {
		return reflect.Value{}, false
	}
	fn := reflect.MakeFunc(want, func(in []reflect.Value) []reflect.Value {
		args := make([]any, len(in))
		for i, arg := range in {
			args[i] = arg.Interface()
		}
		result, err := call(args...)
		if err != nil {
			panic(callbackPanic{err: err})
		}
		if want.NumOut() == 0 {
			return nil
		}
		out, ok := coerceArg(want.Out(0), result)
		if !ok {
			panic(callbackPanic{err: fmt.Errorf("callback returned %s, want %s", phpDebugType(result), want.Out(0))})
		}
		return []reflect.Value{out}
	})
	return fn, true
}

// coerceArg converts v to want where cheap; a callable resolves through coerceArgOn.
func coerceArg(want reflect.Type, v any) (reflect.Value, bool) {
	if want == nil {
		return reflect.ValueOf(v), true
	}
	if v == nil {
		return reflect.Zero(want), true
	}
	rv := reflect.ValueOf(v)
	if rv.Type().AssignableTo(want) {
		return rv, true
	}
	// Duration-taking Go APIs keep their native signature while PHP callers
	// may use Go's readable duration syntax. This applies equally to registered
	// functions and automatically exposed methods and time.Time.Add accepts
	// both a Time\Duration value and a string such as "30m".
	if want == durationType {
		if text, ok := v.(string); ok {
			duration, err := time.ParseDuration(strings.TrimSpace(text))
			if err != nil {
				return reflect.Value{}, false
			}
			return reflect.ValueOf(duration), true
		}
	}
	// A string parameter renders the value the way PHP renders it in a string
	// context. Go's own conversion covers every integer type and means something
	// else: reflect turns int64(65) into "A" where PHP writes "65".
	if want.Kind() == reflect.String {
		return reflect.ValueOf(phpString(v)).Convert(want), true
	}
	if rv.Type().ConvertibleTo(want) {
		return rv.Convert(want), true
	}
	return reflect.Value{}, false
}

// callResult reduces reflect results to value and error, two or more becoming array.
func callResult(out []reflect.Value) (any, error) {
	var values []reflect.Value
	for _, o := range out {
		if o.Type() == errorType {
			err, _ := o.Interface().(error)
			if err != nil {
				return nil, err
			}
			continue
		}
		values = append(values, o)
	}
	switch len(values) {
	case 0:
		return nil, nil
	case 1:
		return values[0].Interface(), nil
	}
	list := model.NewArraySize(len(values))
	for _, v := range values {
		list.Append(v.Interface())
	}
	return list, nil
}
