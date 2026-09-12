package runner

import (
	"sync"

	flatvm "github.com/titpetric/phpscript/flatstack/engine"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner/expr"
)

type compiledExpr struct {
	vars []string
	// idents holds the identifier for each entry of vars (varIdent), built
	// once at compile time; resolveVar reads the bare-name kind off it.
	idents []string
	// calls holds the registered-function names the expression calls, so
	// Eval installs exactly those closures into the evaluation environment
	// instead of the whole function table (see Runtime.installFunc).
	calls    []string
	closures map[string]*model.Closure
	exprs    map[string]model.Expr
	prog     *expr.Program
	// varSlots maps each entry of vars to the engine's slot for its
	// identifier; closureSlots does the same for closure literals. Both are
	// resolved once here so Eval binds by index instead of by map key.
	varSlots     []int
	closureSlots map[string]int
}

// newDirectCompiledExpr adapts a compiled expression to the binding lists
// Eval iterates; see ident.go for the identifier spelling.
func newDirectCompiledExpr(dc *expr.Compiled) *compiledExpr {
	n := len(dc.Vars)
	buf := make([]string, 2*n)
	varSlots := make([]int, n)
	for i, b := range dc.Vars {
		buf[i] = b.Name
		if b.Const {
			buf[n+i] = constIdent(b.Name)
		} else {
			buf[n+i] = varIdent(b.Name)
		}
		varSlots[i] = b.Slot
	}
	ce := &compiledExpr{
		vars:     buf[:n:n],
		idents:   buf[n:],
		varSlots: varSlots,
		calls:    dc.Calls,
		exprs:    dc.Exprs,
		prog:     dc.Program,
	}
	if len(dc.Closures) > 0 {
		ce.closures = make(map[string]*model.Closure, len(dc.Closures))
		ce.closureSlots = make(map[string]int, len(dc.Closures))
		for _, c := range dc.Closures {
			ce.closures[c.ID] = c.Decl
			ce.closureSlots[c.ID] = c.Slot
		}
	}
	return ce
}

// ExprCache stores immutable compiled expressions by AST node identity and
// flat bytecode by parsed program identity. Runtime-local binding metadata
// (compiledExpr) is rebuilt per runtime; both maps retain their key's node
// for the lifetime of the explicitly shared cache. Capacity is bounded to
// prevent memory leaks.
type ExprCache struct {
	mu         sync.RWMutex
	maxEntries int
	byAST      map[*model.Program]*flatvm.Program
	// byExpr caches compiled expressions by AST node identity: a runtime
	// evaluating an expression another runtime compiled reuses the closure
	// chain.
	byExpr map[model.Expr]*expr.Compiled
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
		byAST:      make(map[*model.Program]*flatvm.Program),
		byExpr:     make(map[model.Expr]*expr.Compiled),
	}
}

// Clear resets the cached compiled expressions.
func (c *ExprCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byAST = make(map[*model.Program]*flatvm.Program)
	c.byExpr = make(map[model.Expr]*expr.Compiled)
}

// GetExpr returns the direct-compiled expression cached for e, if any.
func (c *ExprCache) GetExpr(e model.Expr) (*expr.Compiled, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	dc, ok := c.byExpr[e]
	return dc, ok
}

// SetExpr stores a direct-compiled expression by node identity. Evicts one
// item if max capacity is reached.
func (c *ExprCache) SetExpr(e model.Expr, dc *expr.Compiled) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byExpr == nil {
		c.byExpr = make(map[model.Expr]*expr.Compiled)
	}
	limit := c.maxEntries
	if limit <= 0 {
		limit = DefaultMaxCacheSize
	}
	if _, exists := c.byExpr[e]; !exists && len(c.byExpr) >= limit {
		for k := range c.byExpr {
			delete(c.byExpr, k)
			break
		}
	}
	c.byExpr[e] = dc
}

// Len returns the number of currently cached compiled expressions.
func (c *ExprCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.byExpr)
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
