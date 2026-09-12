package expr

import (
	"fmt"

	"github.com/titpetric/phpscript/model"
)

// Env is the engine's evaluation environment. Per-expression values (PHP
// variables, bare-name constants, closure literals) live in Vars, indexed by
// the slot the compiler assigned to their identifier; everything persistent
// (the PHP-semantic helpers, installed functions) stays in Base.
type Env struct {
	Vars []any
	Base map[string]any
}

// closure evaluates one node against the runner's evaluation environment.
type closure = func(env *Env) (any, error)

// Helpers carries the typed implementations of the PHP-semantic helper
// functions. The engine calls them directly, without the []any argument
// slice or a per-call panic guard. Only the pure helpers belong here;
// anything that re-enters the interpreter (__call, __get, __new, registered
// functions) stays an env lookup, because those are per-runtime closures.
type Helpers struct {
	Truthy     func(v any) bool
	Concat     func(a, b any) string
	Pair       func(key, val any) model.ArrayItemValue
	Array      func(items ...model.ArrayItemValue) *model.Array
	Index      func(base, idx any) any
	Cast       func(typ string, v any) any
	Arith      func(op string, a, b any) any
	Compare    func(op string, a, b any) bool
	Bitwise    func(op string, a, b any) (any, error)
	BitNot     func(v any) any
	InstanceOf func(value, class any) bool
	Negate     func(v any) any

	// PanicError converts a recovered panic into the error the runner's own
	// call boundary would have produced, so a host panic stays catchable as
	// the same PHP exception on both engines.
	PanicError func(recovered any) error

	// Slotted reports whether an identifier is a per-evaluation value: a
	// PHP variable, a bare name, a closure literal. Those compile to slot
	// reads; everything else (function names) stays a Base lookup.
	Slotted func(name string) bool
}

// constClosure returns v itself: literals are boxed once at compile time.
func constClosure(v any) closure {
	return func(*Env) (any, error) { return v, nil }
}

// wrapRoot adds the per-evaluation panic guard: one recover per program
// instead of one per helper call. A host panic surfaces as an error;
// PanicError keeps the error type the runner's call boundary produces.
func wrapRoot(body closure, h *Helpers) closure {
	return func(env *Env) (out any, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				out = nil
				if h.PanicError != nil {
					err = h.PanicError(recovered)
				} else {
					err = fmt.Errorf("panic: %v", recovered)
				}
			}
		}()
		return body(env)
	}
}
