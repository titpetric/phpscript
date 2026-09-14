package runner

import (
	"fmt"
	"reflect"

	"github.com/titpetric/phpscript/model"
)

// This file is the one home of Go-callable dispatch. invokeAny used to
// re-derive everything per call: two reflect.TypeOf traversals, the arity
// check, a linear type switch over ~40 signatures, and on a miss a per-
// argument parameter-type walk. newInvoker runs all of that once and hands
// back a pre-bound call: registered functions cache theirs in the function
// table entry, constructors and the evaluation environment's helpers build
// theirs where they are installed, and only a callable that never had a
// registration point still pays the constructor per call.

// invoker is one callable's pre-bound dispatch. call owns the panic
// boundary and the arity check; wantsCtx is the context-injection decision
// invokeWithScopeContext and the flatstack host read without reflecting.
type invoker struct {
	call     func(args []any) (any, error)
	wantsCtx bool
}

// newInvoker builds the invoker for fn: the specialized direct call when
// fn's signature is one of the shapes the stdlib and the evaluation
// environment register, the pre-planned reflect call otherwise.
func newInvoker(fn any) invoker {
	ft := reflect.TypeOf(fn)
	if ft == nil || ft.Kind() != reflect.Func {
		return invoker{call: func([]any) (any, error) {
			return nil, fmt.Errorf("not callable: %T", fn)
		}}
	}
	call := fastInvoker(fn, ft)
	if call == nil {
		call = reflectInvoker(fn, ft)
	}
	return invoker{
		call:     guardInvoker(call, ft, fmt.Sprintf("%T", fn)),
		wantsCtx: wantsContext(ft),
	}
}

// guardInvoker wraps a call with what invokeAny promised every binding:
// a panic arrives as a catchable HostPanicError naming the original
// function's type, and a call passing more arguments than a non-variadic
// signature declares is an ArgumentCountError before anything runs. Too few
// arguments stay legal: a Go binding spells PHP's optional parameters as
// extra ones, and the call paths zero-pad them.
func guardInvoker(call func([]any) (any, error), ft reflect.Type, typeName string) func([]any) (any, error) {
	numIn, variadic := ft.NumIn(), ft.IsVariadic()
	return func(args []any) (result any, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				result = nil
				err = &HostPanicError{Callable: typeName, Value: recovered}
			}
		}()
		if !variadic && len(args) > numIn {
			return nil, &ArgumentCountError{Want: numIn, Got: len(args)}
		}
		return call(args)
	}
}

// reflectInvoker is the fallback constructor: the reflect call with the
// parameter plan - types, variadic element, context flag - computed here
// instead of per argument per call. Coercion itself stays coerceArg, the
// one table.
func reflectInvoker(fn any, ft reflect.Type) func([]any) (any, error) {
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
	return func(args []any) (any, error) {
		in := make([]reflect.Value, 0, len(args))
		// The runtime context, when a binding asks for one, is injected
		// ahead of the script's arguments and does not count towards the
		// PHP position.
		offset := 1
		if wantsCtx {
			offset = 0
		}
		for i, a := range args {
			want := paramAt(i)
			v, ok := coerceArg(a, want)
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

// fastInvoker is invokeFast's type switch run once: each covered signature
// returns a closure calling the binding directly, coercing exactly as the
// reflect path coerces (string parameters through phpString, absent
// arguments as zero values, variadic tails aliased under the borrowed-
// arguments contract). nil sends the constructor to reflectInvoker.
func fastInvoker(fn any, ft reflect.Type) func([]any) (any, error) {
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
		slow := reflectInvoker(fn, ft)
		return func(args []any) (any, error) {
			if flag, ok := argAt(args, 1).(bool); ok {
				return f(phpString(argAt(args, 0)), flag)
			}
			return slow(args)
		}
	case func(string) func(any):
		return func(args []any) (any, error) { return f(phpString(argAt(args, 0))), nil }
	}
	return nil
}

// funcEntry is one function-table slot: the raw callable that introspection
// reflects over, and the invoker built on the entry's first dispatch. Built
// lazily because a server constructs a fresh runtime per request and
// registers the whole stdlib into it; wrapping ~300 bindings eagerly would
// tax every request for the dozen a script calls. Re-registration stores a
// fresh entry, which is the cache invalidation.
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

// invokeEntry dispatches a table entry: context injection decided off the
// pre-bound flag instead of a reflect.TypeOf per call, the pre-bound call,
// and the memory burst guard a host call has always carried.
func (rt *Runtime) invokeEntry(e *funcEntry, args []any, scope *Scope) (any, error) {
	inv := e.invoker()
	if inv.wantsCtx {
		full := make([]any, 0, len(args)+1)
		full = append(full, contextWithScope(contextWithEnv(rt.ctx, rt.Env), scope))
		full = append(full, args...)
		args = full
	}
	result, err := inv.call(args)
	if err == nil && rt.opts.MemoryLimit > 0 {
		// Burst guard: a single host call can allocate far more than the
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
