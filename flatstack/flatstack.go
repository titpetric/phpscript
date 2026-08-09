// Package flatstack provides the runner embedding API with an opt-in flat
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

type Runtime = runner.Runtime
type Options = runner.Options
type ExitError = runner.ExitError
type HostPanicError = runner.HostPanicError
type ExprCache = runner.ExprCache
type IncludeCache = runner.IncludeCache
type IncludeFunc = runner.IncludeFunc
type Scope = runner.Scope
type Context = runner.Context
type Transpiler = runner.Transpiler

func New(w io.Writer, opts Options) *Runtime { return runner.NewFlatStack(w, opts) }
func NewExprCache() *ExprCache               { return runner.NewExprCache() }
func NewIncludeCache() *IncludeCache         { return runner.NewIncludeCache() }
func NewScope() *Scope                       { return runner.NewScope() }
func NewContext() Context                    { return runner.NewContext() }
func FromRequest(r *http.Request) Context    { return runner.FromRequest(r) }
func NewTranspiler() *Transpiler             { return runner.NewTranspiler() }

func ScopeFromContext(ctx context.Context) (*Scope, bool) {
	return runner.ScopeFromContext(ctx)
}

func IsExit(err error) (*ExitError, bool) { return runner.IsExit(err) }

// Supports reports whether p will execute through flat bytecode rather than
// the compatibility interpreter. Call this in benchmarks to prevent measuring
// an accidental fallback.
func Supports(p *model.Program) error {
	_, err := flatvm.Compile(p)
	return err
}
