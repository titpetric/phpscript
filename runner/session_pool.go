package runner

import (
	"context"
	"io"
	goruntime "runtime"
	"sync"
)

// SessionPool keeps registered runtimes alive between the sessions they
// serve. A host answering one request after another pays registration once,
// plus a reset per request. It uses a bounded free list, not sync.Pool:
// the registered runtime is the asset worth keeping, and sync.Pool drops
// entries on a collection. A Get on an empty list builds through the
// constructor; a Put past the bound drops. One pool serves one host. The
// constructor captures a source root, plus the options. A server hosting more
// than one virtual host keeps a pool per site. A runtime registered for
// a site's filesystem never answers for another's.
type SessionPool struct {
	mu    sync.Mutex
	free  []*Runtime
	build func() *Runtime
	size  int
}

// NewSessionPool returns a pool of up to size runtimes built by build. Zero
// size is GOMAXPROCS, the parallelism available for work that is all CPU, the
// same default Pool takes for its worker count.
func NewSessionPool(size int, build func() *Runtime) *SessionPool {
	if size <= 0 {
		size = goruntime.GOMAXPROCS(0)
	}
	return &SessionPool{
		free:  make([]*Runtime, 0, size),
		build: build,
		size:  size,
	}
}

// Get returns a reset runtime, building one when the list is empty.
func (p *SessionPool) Get() *Runtime {
	p.mu.Lock()
	if n := len(p.free) - 1; n >= 0 {
		rt := p.free[n]
		p.free[n] = nil
		p.free = p.free[:n]
		p.mu.Unlock()
		return rt
	}
	p.mu.Unlock()
	return p.build()
}

// Put resets rt and returns it to the list, dropping it when the list is full.
//
// The reset happens here and not at the next Get: an idle runtime holds nothing
// of the request it answered.
func (p *SessionPool) Put(rt *Runtime) {
	// A frame still on the stack means a run this runtime started never
	// returned, and no reset covers that state and this drops the runtime and
	// the next Get builds a sound one.
	if rt == nil || len(rt.frames) != 0 {
		return
	}
	// The request that installed the collector folded it into its aggregator;
	// left behind it would double-count the next request.
	rt.coverage = nil
	rt.ResetSession(io.Discard, nil)
	if !rt.opts.Precompile {
		// Without precompilation the runtime parses the entrypoint per
		// request and AST nodes no cache holds key these maps and they would
		// grow by one program per request. Under precompilation the include
		// cache owns the nodes and the tree it caches bounds the maps, the
		// reuse this pool exists for.
		clear(rt.compiled)
		clear(rt.concatParts)
		clear(rt.sourceSpans)
	}
	// Detached from the request it served and an idle runtime keeps no
	// connection alive; the next session installs its own.
	rt.SetContext(context.Background())
	p.mu.Lock()
	if len(p.free) < p.size {
		p.free = append(p.free, rt)
	}
	p.mu.Unlock()
}
