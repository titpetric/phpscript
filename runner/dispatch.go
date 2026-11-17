package runner

import (
	"fmt"
	"reflect"

	"github.com/titpetric/phpscript/model"
)

// This file is the one home of Go-callable dispatch. invokeAny used to
// re-derive everything per call: two reflect.TypeOf traversals, the arity
// check, a linear type switch over ~40 signatures, and on a miss a per-
// argument parameter-type walk. newInvoker runs that once and returns a
// pre-bound call: registered functions cache theirs in the function table
// entry, constructors and the evaluation environment's helpers build theirs
// at the point that installs them, and only a callable that never had a
// registration point still pays the constructor per call.

// invoker is one callable's pre-bound dispatch, with call taking the calling runtime.
type invoker struct {
	call     func(rt *Runtime, args []any) (any, error)
	wantsCtx bool
}

// newInvoker builds the invoker for fn, preferring a specialized direct call.
func newInvoker(fn any) invoker {
	// A callable value is not a Go func and has no signature to plan: the
	// runtime doing the calling dispatches it.
	if c, ok := fn.(*Callable); ok {
		return invoker{call: func(rt *Runtime, args []any) (any, error) { return c.on(rt)(args...) }}
	}
	ft := reflect.TypeOf(fn)
	if ft == nil || ft.Kind() != reflect.Func {
		return invoker{call: func(*Runtime, []any) (any, error) {
			return nil, fmt.Errorf("not callable: %T", fn)
		}}
	}
	call := reflectInvoker(ft, fn)
	if fast := fastInvoker(ft, fn); fast != nil {
		call = func(_ *Runtime, args []any) (any, error) { return fast(args) }
	}
	return invoker{
		call:     guardInvoker(call, ft, fmt.Sprintf("%T", fn)),
		wantsCtx: wantsContext(ft),
	}
}

// guardInvoker turns a panic into HostPanicError and too many arguments into ArgumentCountError.
func guardInvoker(call func(*Runtime, []any) (any, error), ft reflect.Type, typeName string) func(*Runtime, []any) (any, error) {
	numIn, variadic := ft.NumIn(), ft.IsVariadic()
	injected := injectedArgs(ft)
	return func(rt *Runtime, args []any) (result any, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				result = nil
				err = recoveredError(recovered, typeName)
			}
		}()
		if !variadic && len(args) > numIn {
			return nil, &ArgumentCountError{Want: numIn - injected, Got: len(args) - injected}
		}
		return call(rt, args)
	}
}

// reflectInvoker plans the parameter types once and calls fn through reflect.
func reflectInvoker(ft reflect.Type, fn any) func(*Runtime, []any) (any, error) {
	rv := reflect.ValueOf(fn)
	numIn, variadic := ft.NumIn(), ft.IsVariadic()
	wantsCtx := wantsContext(ft)
	params := make([]reflect.Type, numIn)
	for i := range params {
		params[i] = ft.In(i)
	}
	var elem reflect.Type
	if variadic {
		elem = params[numIn-1].Elem()
	}
	paramAt := func(i int) reflect.Type {
		if variadic && i >= numIn-1 {
			return elem
		}
		if i < numIn {
			return params[i]
		}
		return nil
	}
	return func(rt *Runtime, args []any) (any, error) {
		in := make([]reflect.Value, 0, len(args))
		// A binding that declares a runtime context takes it ahead of the
		// script's arguments. That slot does not count towards the PHP
		// position.
		offset := 1
		if wantsCtx {
			offset = 0
		}
		for i, a := range args {
			want := paramAt(i)
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
		return callResult(rv.Call(in))
	}
}

// fastInvoker returns a direct call for a covered signature and nil for the rest.
func fastInvoker(ft reflect.Type, fn any) func([]any) (any, error) {
	switch f := fn.(type) {
	case func(...any) (any, error):
		return func(args []any) (any, error) { return f(args...) }
	case func(any) any:
		return func(args []any) (any, error) { return f(argAt(args, 0)), nil }
	case func(any) bool:
		return func(args []any) (any, error) { return f(argAt(args, 0)), nil }
	case func(any) string:
		return func(args []any) (any, error) { return f(argAt(args, 0)), nil }
	case func(any, any) string:
		return func(args []any) (any, error) { return f(argAt(args, 0), argAt(args, 1)), nil }
	case func(any, any) (string, error):
		return func(args []any) (any, error) { return f(argAt(args, 0), argAt(args, 1)) }
	case func(any, any) any:
		return func(args []any) (any, error) { return f(argAt(args, 0), argAt(args, 1)), nil }
	case func(string) string:
		return func(args []any) (any, error) { return f(phpString(argAt(args, 0))), nil }
	case func() string:
		return func([]any) (any, error) { return f(), nil }
	case func() any:
		return func([]any) (any, error) { return f(), nil }
	case func(...any) any:
		return func(args []any) (any, error) { return f(args...), nil }
	case func(...any) bool:
		return func(args []any) (any, error) { return f(args...), nil }
	case func(string) any:
		return func(args []any) (any, error) { return f(phpString(argAt(args, 0))), nil }
	case func(string) bool:
		return func(args []any) (any, error) { return f(phpString(argAt(args, 0))), nil }
	case func(string) int64:
		return func(args []any) (any, error) { return f(phpString(argAt(args, 0))), nil }
	case func() int64:
		return func([]any) (any, error) { return f(), nil }
	case func(string, string) bool:
		return func(args []any) (any, error) {
			return f(phpString(argAt(args, 0)), phpString(argAt(args, 1))), nil
		}
	case func(string, string) string:
		return func(args []any) (any, error) {
			return f(phpString(argAt(args, 0)), phpString(argAt(args, 1))), nil
		}
	case func(string, string) (bool, error):
		return func(args []any) (any, error) {
			return f(phpString(argAt(args, 0)), phpString(argAt(args, 1)))
		}
	case func(any) int64:
		return func(args []any) (any, error) { return f(argAt(args, 0)), nil }
	case func(any) float64:
		return func(args []any) (any, error) { return f(argAt(args, 0)), nil }
	case func(any) (any, error):
		return func(args []any) (any, error) { return f(argAt(args, 0)) }
	case func(any) (bool, error):
		return func(args []any) (any, error) { return f(argAt(args, 0)) }
	case func(any, any) (bool, error):
		return func(args []any) (any, error) { return f(argAt(args, 0), argAt(args, 1)) }
	case func(any, ...any) (any, error):
		return func(args []any) (any, error) { return f(argAt(args, 0), argsTail(args)...) }
	case func(any, ...any) *model.Array:
		return func(args []any) (any, error) { return f(argAt(args, 0), argsTail(args)...), nil }
	case func(string, ...any) string:
		return func(args []any) (any, error) {
			return f(phpString(argAt(args, 0)), argsTail(args)...), nil
		}
	case func(string, ...any) (string, error):
		return func(args []any) (any, error) {
			return f(phpString(argAt(args, 0)), argsTail(args)...)
		}
	case func(string, ...string) string:
		return func(args []any) (any, error) {
			rest := argsTail(args)
			tail := make([]string, len(rest))
			for i, v := range rest {
				tail[i] = phpString(v)
			}
			return f(phpString(argAt(args, 0)), tail...), nil
		}
	case func() *model.Array:
		return func([]any) (any, error) { return f(), nil }
	case func(any, any, ...any) (any, error):
		return func(args []any) (any, error) {
			return f(argAt(args, 0), argAt(args, 1), argsTail2(args)...)
		}
	case func(any, string) any:
		return func(args []any) (any, error) {
			return f(argAt(args, 0), phpString(argAt(args, 1))), nil
		}
	case func(string, any) (any, error):
		return func(args []any) (any, error) { return f(phpString(argAt(args, 0)), argAt(args, 1)) }
	case func(string, any, ...any) (any, error):
		return func(args []any) (any, error) {
			return f(phpString(argAt(args, 0)), argAt(args, 1), argsTail2(args)...)
		}
	case func(string, string, ...any) (any, error):
		return func(args []any) (any, error) {
			return f(phpString(argAt(args, 0)), phpString(argAt(args, 1)), argsTail2(args)...)
		}
	case func(string, string) (any, error):
		return func(args []any) (any, error) {
			return f(phpString(argAt(args, 0)), phpString(argAt(args, 1)))
		}
	case func(string, bool) (any, error):
		// The bool argument's shape is only known per call; a mismatch takes
		// the reflect plan so the TypeError path stays identical.
		slow := reflectInvoker(ft, fn)
		return func(args []any) (any, error) {
			if flag, ok := argAt(args, 1).(bool); ok {
				return f(phpString(argAt(args, 0)), flag)
			}
			return slow(nil, args)
		}
	case func(string) func(any):
		return func(args []any) (any, error) { return f(phpString(argAt(args, 0))), nil }
	}
	return nil
}

// funcEntry is one function-table slot, holding its invoker built on first dispatch.
type funcEntry struct {
	fn    any
	inv   invoker
	bound bool
}

func (e *funcEntry) invoker() *invoker {
	if !e.bound {
		e.inv = newInvoker(e.fn)
		e.bound = true
	}
	return &e.inv
}

// invokeEntry dispatches a table entry, injecting context and checking the memory burst.
func (rt *Runtime) invokeEntry(e *funcEntry, args []any, scope *Scope) (any, error) {
	inv := e.invoker()
	if inv.wantsCtx {
		full := make([]any, 0, len(args)+1)
		full = append(full, rt.bindingContext(scope))
		full = append(full, args...)
		args = full
	}
	result, err := inv.call(rt, args)
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
