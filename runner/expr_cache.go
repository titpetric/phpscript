package runner

import (
	"sync"

	"github.com/expr-lang/expr/vm"
	"github.com/titpetric/phpscript/model"
)

type compiledExpr struct {
	src      string
	vars     []string
	closures map[string]*model.Closure
	exprs    map[string]model.Expr
	prog     *vm.Program
}

// ExprCache stores immutable compiled expression programs by transpiled source.
// AST-specific metadata stays on each Runtime so a shared cache neither retains
// freshly parsed request ASTs nor reuses closures/nested expressions from a
// different program.
type ExprCache struct {
	mu    sync.RWMutex
	bySrc map[string]*vm.Program
}

// NewExprCache returns an empty compiled expression cache.
func NewExprCache() *ExprCache {
	return &ExprCache{}
}

// GetSource returns the compiled expression cached for src, if any.
func (c *ExprCache) GetSource(src string) (*vm.Program, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	prog, ok := c.bySrc[src]
	return prog, ok
}

// SetSource stores a compiled program for transpiled source.
func (c *ExprCache) SetSource(src string, prog *vm.Program) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bySrc == nil {
		c.bySrc = make(map[string]*vm.Program)
	}
	c.bySrc[src] = prog
}
