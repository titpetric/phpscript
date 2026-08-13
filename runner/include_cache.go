package runner

import (
	"sync"

	"github.com/titpetric/phpscript/model"
)

// DefaultMaxCacheSize is the default upper bound on cached entries.
const DefaultMaxCacheSize = 10000

// IncludeCache stores parsed include/require programs by cleaned filesystem
// path. Parsed programs are treated as immutable by Runtime.Run/exec: hoisting
// copies declarations into runtime maps, while statement execution only reads
// the AST, so cached *model.Program values can be shared safely by callers that
// do not mutate ASTs themselves. Cache size is bounded to prevent memory leaks.
type IncludeCache struct {
	mu         sync.RWMutex
	maxEntries int
	programs   map[string]*model.Program
}

// NewIncludeCache returns an empty parsed include cache with default capacity (10,000 entries).
func NewIncludeCache() *IncludeCache {
	return NewIncludeCacheWithCapacity(DefaultMaxCacheSize)
}

// NewIncludeCacheWithCapacity returns an empty include cache bounded to maxEntries.
func NewIncludeCacheWithCapacity(maxEntries int) *IncludeCache {
	if maxEntries <= 0 {
		maxEntries = DefaultMaxCacheSize
	}
	return &IncludeCache{
		maxEntries: maxEntries,
		programs:   make(map[string]*model.Program),
	}
}

// Clear resets the cached entries.
func (c *IncludeCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.programs = make(map[string]*model.Program)
}

// Len returns the number of currently cached programs.
func (c *IncludeCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.programs)
}

// Get returns the parsed program cached for path, if any.
func (c *IncludeCache) Get(path string) (*model.Program, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	prog, ok := c.programs[path]
	return prog, ok
}

// Set stores prog for path. Evicts one item if max capacity is reached.
func (c *IncludeCache) Set(path string, prog *model.Program) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.programs == nil {
		c.programs = make(map[string]*model.Program)
	}
	limit := c.maxEntries
	if limit <= 0 {
		limit = DefaultMaxCacheSize
	}
	if _, exists := c.programs[path]; !exists && len(c.programs) >= limit {
		for k := range c.programs {
			delete(c.programs, k)
			break
		}
	}
	c.programs[path] = prog
}
