// Package expr is the runner's seam to the expression engine. Everything the
// runner knows about github.com/expr-lang/expr passes through here: the
// compiled program type, the compile configuration, the nature machinery the
// type env is built from, and the run entry point. An engine change is a
// change to this package, not to the runner.
package expr

import (
	"errors"

	upstream "github.com/expr-lang/expr"
	"github.com/expr-lang/expr/checker"
	"github.com/expr-lang/expr/checker/nature"
	"github.com/expr-lang/expr/compiler"
	"github.com/expr-lang/expr/conf"
	"github.com/expr-lang/expr/file"
	"github.com/expr-lang/expr/optimizer"
	"github.com/expr-lang/expr/vm"
)

// Aliases, not wrappers: the runner reads and writes Config fields and hands
// Nature values back to the checker, so the identities must match upstream's
// exactly. Bytecode is upstream's program type, kept nameable because the
// reference surface below still produces one.
type (
	Bytecode    = vm.Program
	Config      = conf.Config
	Nature      = nature.Nature
	NatureCache = nature.Cache
	TypeData    = nature.TypeData
	Option      = upstream.Option
)

// Program is one compiled expression: the bytecode the VM runs and, when the
// closure engine recognises the expression's shape, a closure chain that
// evaluates it without the VM. The bytecode is always present; it is what
// Disassemble reports and what Run falls back to, so an expression the
// closure engine declines loses nothing.
type Program struct {
	vm  *Bytecode
	run func(env map[string]any) (any, error)
}

// Disassemble reports the program's bytecode.
func (p *Program) Disassemble() string {
	return p.vm.Disassemble()
}

// Run evaluates a compiled program against env. The closure engine only ever
// sees the runner's map environment; any other env shape runs on the VM.
func Run(p *Program, env any) (any, error) {
	if p.run != nil {
		if m, ok := env.(map[string]any); ok {
			return p.run(m)
		}
	}
	return upstream.Run(p.vm, env)
}

// NewConfig returns an empty compile configuration.
func NewConfig() *Config {
	return conf.CreateNew()
}

// CompileWith runs expr's parse/check/optimize/compile pipeline against a
// prebuilt config. It mirrors expr.Compile, which cannot be used here because
// it insists on constructing a fresh conf.Config (and re-deriving the type
// env) on every call.
func CompileWith(src string, c *Config) (*Program, error) {
	tree, err := checker.ParseCheck(src, c)
	if err != nil {
		return nil, err
	}
	if c.Optimize {
		if err := optimizer.Optimize(&tree.Node, c); err != nil {
			var fileError *file.Error
			if errors.As(err, &fileError) {
				return nil, fileError.Bind(tree.Source)
			}
			return nil, err
		}
	}
	prog, err := compiler.Compile(tree, c)
	if err != nil {
		return nil, err
	}
	return &Program{vm: prog}, nil
}

// The upstream reference surface. The compile guard tests compare the hoisted
// CompileWith pipeline against what expr.Compile emits from a full per-call
// type env; that comparison is only meaningful against upstream itself, so
// these stay thin forwards whatever the engine behind Run does.

// Compile compiles src with upstream expr.Compile.
func Compile(src string, opts ...Option) (*Bytecode, error) {
	return upstream.Compile(src, opts...)
}

// Env is upstream expr.Env: derive the compile-time type env from v.
func Env(v any) Option {
	return upstream.Env(v)
}

// DisableAllBuiltins is upstream expr.DisableAllBuiltins.
func DisableAllBuiltins() Option {
	return upstream.DisableAllBuiltins()
}

// EnvWithCache is upstream conf.EnvWithCache: the reflective nature walk the
// runner's hand-built type env is pinned against.
func EnvWithCache(c *NatureCache, env any) Nature {
	return conf.EnvWithCache(c, env)
}
