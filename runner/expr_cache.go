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

// ExprCache stores compiled expression programs by AST expression node and by
// transpiled expr source. The node cache is the fastest path for reused parsed
// programs. The source cache lets freshly parsed but identical PHP expression
// trees reuse compiled expr bytecode across load+execute cycles.
type ExprCache struct {
	mu    sync.Mutex
	exprs map[model.Expr]*compiledExpr
	bySrc map[string]*compiledExpr
}

// NewExprCache returns an empty compiled expression cache.
func NewExprCache() *ExprCache {
	return &ExprCache{exprs: map[model.Expr]*compiledExpr{}, bySrc: map[string]*compiledExpr{}}
}

// Get returns the compiled expression cached for e, if any.
func (c *ExprCache) Get(e model.Expr) (*compiledExpr, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ce, ok := c.exprs[e]
	return ce, ok
}

// Set stores ce for e and its transpiled source.
func (c *ExprCache) Set(e model.Expr, ce *compiledExpr) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.exprs[e] = ce
	c.bySrc[ce.src] = ce
}

// GetSource returns the compiled expression cached for src, if any.
func (c *ExprCache) GetSource(src string) (*compiledExpr, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ce, ok := c.bySrc[src]
	return ce, ok
}
