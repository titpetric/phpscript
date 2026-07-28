package runner

import (
	"sync"

	"github.com/titpetric/phpscript/model"
)

// IncludeCache stores parsed include/require programs by cleaned filesystem
// path. Parsed programs are treated as immutable by Runtime.Run/exec: hoisting
// copies declarations into runtime maps, while statement execution only reads
// the AST, so cached *model.Program values can be shared safely by callers that
// do not mutate ASTs themselves.
type IncludeCache struct {
	mu       sync.RWMutex
	programs map[string]*model.Program
}

// NewIncludeCache returns an empty parsed include cache.
func NewIncludeCache() *IncludeCache {
	return &IncludeCache{}
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

// Set stores prog for path.
func (c *IncludeCache) Set(path string, prog *model.Program) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.programs == nil {
		c.programs = make(map[string]*model.Program)
	}
	c.programs[path] = prog
}
