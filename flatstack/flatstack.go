// Package flatstack is the runner embedding API with an opt-in flat
// bytecode backend. Unsupported programs are rejected before execution and run
// by the full interpreter, preserving runner behavior while the bytecode subset
// grows.
package flatstack

import (
	"context"
	"io"
	"net/http"

	flatvm "github.com/titpetric/phpscript/flatstack/engine"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// The embedding API is runner's, aliased and never wrapped: a host that swaps
// runner for flatstack changes its import and nothing else, and a value built
// here is the same value either package would hand back. Only New differs, and
// and this package decides nothing else.

// Runtime is runner.Runtime.
type Runtime = runner.Runtime

// Options is runner.Options.
type Options = runner.Options

// ExitError is runner.ExitError.
type ExitError = runner.ExitError

// HostPanicError is runner.HostPanicError.
type HostPanicError = runner.HostPanicError

// ExprCache is runner.ExprCache.
type ExprCache = runner.ExprCache

// IncludeCache is runner.IncludeCache.
type IncludeCache = runner.IncludeCache

// IncludeFunc is runner.IncludeFunc.
type IncludeFunc = runner.IncludeFunc

// Scope is runner.Scope.
type Scope = runner.Scope

// Context is runner.Context.
type Context = runner.Context

// New returns a runtime that executes a supported program through flat bytecode
// and delegates the whole of an unsupported one to the interpreter. It is the one
// call that separates this package from runner.
func New(w io.Writer, opts Options) *Runtime { return runner.NewFlatStack(w, opts) }

// NewExprCache returns a compiled-expression cache to share between runtimes.
func NewExprCache() *ExprCache { return runner.NewExprCache() }

// NewIncludeCache returns a parsed-program cache to share between runtimes.
func NewIncludeCache() *IncludeCache { return runner.NewIncludeCache() }

// NewScope returns an empty scope.
func NewScope() *Scope { return runner.NewScope() }

// NewContext returns a request context a host fills itself.
func NewContext() Context { return runner.NewContext() }

// FromRequest returns the request context of r, decoded as a script reads it.
func FromRequest(r *http.Request) Context { return runner.FromRequest(r) }

// ScopeFromContext answers the PHP scope a binding was called from, when one is
// on ctx.
func ScopeFromContext(ctx context.Context) (*Scope, bool) {
	return runner.ScopeFromContext(ctx)
}

// IsExit reports whether err is the error exit() and die() raise, and answers it.
func IsExit(err error) (*ExitError, bool) { return runner.IsExit(err) }

// Supports reports whether p will execute through flat bytecode and not
// the compatibility interpreter. Call this in benchmarks to prevent measuring
// an accidental fallback.
func Supports(p *model.Program) error {
	_, err := flatvm.Compile(p)
	return err
}
