package runner

import (
	"sync"

	"github.com/expr-lang/expr/vm"

	flatvm "github.com/titpetric/phpscript/flatstack/engine"
	"github.com/titpetric/phpscript/model"
)

type compiledExpr struct {
	src  string
	vars []string
	// idents holds the expr identifier for each entry of vars (varIdent), built
	// once at compile time so Eval does not rebuild the "v_"-prefixed strings on
	// every evaluation.
	idents []string
	// calls holds the registered-function names the expression calls as bare env
	// identifiers, so Eval can install exactly those closures into the evaluation
	// environment instead of the whole function table (see Runtime.buildEnv).
	calls    []string
	closures map[string]*model.Closure
	exprs    map[string]model.Expr
	prog     *vm.Program
}

// newCompiledExpr snapshots one compiled expression. vars, idents and calls come
// from the pooled transpiler and are copied into a single backing array here,
// both because the transpiler reuses its own storage and because one allocation
// is cheaper than three.
func newCompiledExpr(src string, vars, idents, calls []string, closures map[string]*model.Closure, exprs map[string]model.Expr, prog *vm.Program) *compiledExpr {
	n := len(vars)
	c := len(calls)
	buf := make([]string, 2*n+c)
	copy(buf, vars)
	copy(buf[n:], idents)
	copy(buf[2*n:], calls)
	return &compiledExpr{
		src:      src,
		vars:     buf[:n:n],
		idents:   buf[n : 2*n : 2*n],
		calls:    buf[2*n:],
		closures: closures,
		exprs:    exprs,
		prog:     prog,
	}
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
