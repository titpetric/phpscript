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
// of the explicitly shared cache. Cache capacity is bounded to prevent memory leaks.
type ExprCache struct {
	mu         sync.RWMutex
	maxEntries int
	bySrc      map[string]*vm.Program
	byAST      map[*model.Program]*flatvm.Program
}

// NewExprCache returns an empty compiled expression cache with default capacity (10,000 entries).
func NewExprCache() *ExprCache {
	return NewExprCacheWithCapacity(DefaultMaxCacheSize)
}

// NewExprCacheWithCapacity returns an empty expression cache bounded to maxEntries.
func NewExprCacheWithCapacity(maxEntries int) *ExprCache {
	if maxEntries <= 0 {
		maxEntries = DefaultMaxCacheSize
	}
	return &ExprCache{
		maxEntries: maxEntries,
		bySrc:      make(map[string]*vm.Program),
		byAST:      make(map[*model.Program]*flatvm.Program),
	}
}

// Clear resets the cached compiled expressions.
func (c *ExprCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bySrc = make(map[string]*vm.Program)
	c.byAST = make(map[*model.Program]*flatvm.Program)
}

// Len returns the number of currently cached source expressions.
func (c *ExprCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.bySrc)
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
	limit := c.maxEntries
	if limit <= 0 {
		limit = DefaultMaxCacheSize
	}
	if _, exists := c.byAST[p]; !exists && len(c.byAST) >= limit {
		for k := range c.byAST {
			delete(c.byAST, k)
			break
		}
	}
	c.byAST[p] = program
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

// SetSource stores a compiled program for transpiled source. Evicts one item if max capacity is reached.
func (c *ExprCache) SetSource(src string, prog *vm.Program) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bySrc == nil {
		c.bySrc = make(map[string]*vm.Program)
	}
	limit := c.maxEntries
	if limit <= 0 {
		limit = DefaultMaxCacheSize
	}
	if _, exists := c.bySrc[src]; !exists && len(c.bySrc) >= limit {
		for k := range c.bySrc {
			delete(c.bySrc, k)
			break
		}
	}
	c.bySrc[src] = prog
}
