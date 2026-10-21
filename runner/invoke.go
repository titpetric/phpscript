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
// sees. The pre-bound dispatch it feeds lives in dispatch.go.

// contextType is context.Context as a reflect.Type, which detects the callables
// that declare the runtime context as their first argument for auto-injection.
var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()

var errorType = reflect.TypeOf((*error)(nil)).Elem()

var durationType = reflect.TypeOf(time.Duration(0))

// callbackPanic carries the error a PHP callback reported out of the Go frames
// between it and the host boundary. A binding declaring func(string) string
// leaves nowhere to return one, so the only way across is a panic; the boundary
// unwraps this one in place of reporting a host panic, so a script that throws
// inside a callback catches its own exception.
type callbackPanic struct{ err error }

// recoveredError turns a recovered value into the error a script sees. callable
// names what panicked, for the host-panic case.
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

// injectedArgs is how many arguments the call paths prepend to the ones a
// script wrote: one for a binding declaring the runtime context, none
// otherwise. Every number a script reads subtracts it, because the context is
// no PHP parameter. Counting it names a limit one higher than the signature
// accepts, and a call at that limit fails with the same message.
//
// A binding that declares the context always arrives with it in place:
// invokeWithScopeContext and invokeEntry prepend it, and callGoMethod prepends
// it for a method, each ahead of the arity check that reads this.
func injectedArgs(t reflect.Type) int {
	if wantsContext(t) {
		return 1
	}
	return 0
}

// invokeWithScopeContext derives a context containing the active PHP frame so
// language helpers such as compact() can inspect caller-local variables and
// observability can identify the active source file.
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

// adapt wraps any Go callable in the uniform func(...any) (any, error)
// signature the evaluation environment carries, which is the one shape the
// engine's call closures assert. The wrapper performs PHP-ish argument
// coercion via reflection so shims keep natural Go signatures.
func adapt(fn any) func(...any) (any, error) {
	inv := newInvoker(fn)
	return func(args ...any) (any, error) { return inv.call(nil, args) }
}

// adaptOn is adapt for a wrapper whose runtime the caller holds. A Go func
// declaring a callable parameter gets one bound to the runtime doing the
// calling, which lets a callable handed across a fork run where the call sits;
// the helpers adapt() wraps take `any` and resolve their own.
func adaptOn(rt *Runtime, fn any) func(...any) (any, error) {
	inv := newInvoker(fn)
	return func(args ...any) (any, error) { return inv.call(rt, args) }
}

// nameCallError fills in the PHP name of an error raised while building a call.
// invokeAny works from the Go signature alone and has no name to report; only
// the dispatch site holds the name a script typed.
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

// phpParamTypeName spells a declared Go parameter type the way PHP names the
// type in a TypeError ("must be of type int"). It describes a *parameter*, so
// it stays separate from phpDebugType, which describes a value, and from
// gettype's legacy names (integer/double/boolean) in stdlib: nothing may fold
// one table into the other.
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

// phpDebugType names a value the way PHP's get_debug_type() does ("array
// given"). It describes a *value*, so it stays separate from phpParamTypeName,
// which describes a declared parameter type, and from gettype's legacy names
// (integer/double/boolean) in stdlib: nothing may fold one table into the
// other.
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

// argAt returns the i-th argument, or nil when the script passed fewer
// arguments than the binding declares. PHP tolerates the short call, and the
// reflect path pads the same way with zero values.
func argAt(args []any, i int) any {
	if i < len(args) {
		return args[i]
	}
	return nil
}

// argsTail2 is the variadic tail after two fixed parameters, aliased the way
// argsTail aliases.
func argsTail2(args []any) []any {
	if len(args) > 2 {
		return args[2:]
	}
	return nil
}

// argsTail is the variadic tail after the first fixed parameter. It aliases
// the slice and copies nothing: each call builds args fresh, so the binding
// sees exactly what reflect.Value.Call would have handed it.
func argsTail(args []any) []any {
	if len(args) > 1 {
		return args[1:]
	}
	return nil
}

// invokeAny calls fn (any Go callable) with args, coercing arguments to the
// declared parameter types where convertible. fastInvoker dispatches the
// common signatures directly; the rest go through reflection.
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

// coerceArgOn converts an argument of a call on rt.
//
// The runtime matters for one kind of value: a callable. A binding declaring the
// uniform callable shape gets one bound to rt and not to the runtime that
// built the value, so a comparator passed to usort or a callback passed to
// array_map inside a handler runs on the worker answering the request and never
// reaches back into the runtime the script built the value on.
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

// adaptCallable wraps a PHP callable in want, the Go function type a binding
// declared.
//
// want must return one value or none, since a PHP closure returns one. An error
// the callable reports has nowhere to go in a signature that declares no error
// slot, so adaptCallable panics with it, and the host boundary turns that into
// a catchable throwable the way it does any other panic in a binding.
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

// coerceArg converts a value to the target parameter type where a cheap
// conversion makes it assignable. The final return reports whether it did: a
// false refuses the value, and the caller turns that into a PHP TypeError,
// with no reflect.Value.Call left to panic on it.
//
// It converts everything except a callable, which needs the runtime the call is
// on and resolves through coerceArgOn: a map key, a slice element and a struct
// field reach this one, none of which a callable arrives as, and keeping it
// callable-free is also what keeps the expression helpers out of an
// initialisation cycle with the interpreter.
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
	// functions and automatically exposed methods, so time.Time.Add accepts
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

// callResult reduces a reflect Call result to (value, error) following Go
// conventions: the trailing error goes to the error result, and the non-error
// values become the PHP result.
//
// A Go callable returning two or more values is not a special case to hide.
// One value returns as itself; two or more become a PHP list, in declaration
// order, which a script destructures the way PHP returns a tuple:
//
//	list($year, $week) = $t->iso_week();
//
// Keeping only the first value would silently answer a different question than
// the one asked: time.Time.ISOWeek returns (year, week), and dropping the tail
// makes `$t->iso_week()` answer the year.
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
