package runner

import (
	"sync"

	"github.com/expr-lang/expr/vm"
	flatvm "github.com/titpetric/phpscript/flatstack/engine"
	"github.com/titpetric/phpscript/model"
)

type compiledExpr struct {
	src      string
	vars     []string
	closures map[string]*model.Closure
	exprs    map[string]model.Expr
	prog     *vm.Program
}

// ExprCache stores immutable compiled expression programs by transpiled source
// and optional flat bytecode by parsed program identity. Expression AST metadata
// stays runtime-local; flat bytecode retains its source Program for the lifetime
// of the explicitly shared cache.
type ExprCache struct {
	mu    sync.RWMutex
	bySrc map[string]*vm.Program
	byAST map[*model.Program]*flatvm.Program
}

func (c *ExprCache) getFlat(p *model.Program) (*flatvm.Program, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	program, ok := c.byAST[p]
	return program, ok
}

func (c *ExprCache) setFlat(p *model.Program, program *flatvm.Program) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byAST == nil {
		c.byAST = make(map[*model.Program]*flatvm.Program)
	}
	c.byAST[p] = program
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
