package runner

import (
	"context"
	"io"
	goruntime "runtime"
	"sync"
)

// SessionPool keeps registered runtimes alive between the sessions they
// serve, so a host answering one request after another pays registration once
// and a reset per request. It is a bounded free list and no sync.Pool: a
// registered runtime is the asset being kept, and sync.Pool drops entries on
// a collection. A Get on an empty list builds through the constructor and a
// Put over the bound drops. One pool serves one host: the constructor closes
// over one source root and one set of options, so a server with several
// virtual hosts keeps a pool per site and a runtime registered against one
// site's filesystem never answers for another's.
type SessionPool struct {
	mu    sync.Mutex
	free  []*Runtime
	build func() *Runtime
	size  int
}

// NewSessionPool returns a pool of up to size runtimes built by build. Zero
// size is GOMAXPROCS, the parallelism the host has for work that is all CPU,
// the same default Pool takes for its worker count.
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
// The reset happens here and not at the next Get, so the values a session
// built are released when it ends and an idle runtime holds nothing of the
// request it answered: ResetSession drops the session's declarations and
// state, the runtime-local caches keyed by AST nodes are dropped unless the
// include cache owns the nodes, and the context is detached from the request
// it served.
func (p *SessionPool) Put(rt *Runtime) {
	// A frame still on the stack means a run this runtime started never
	// returned, which no reset is written for; the runtime is dropped and the
	// next Get builds a sound one.
	if rt == nil || len(rt.frames) != 0 {
		return
	}
	// The collector was folded into its aggregator by the request that
	// installed it; left behind it would double-count the next request.
	rt.coverage = nil
	rt.ResetSession(io.Discard, nil)
	if !rt.opts.Precompile {
		// Without precompilation the entrypoint is parsed per request, so
		// these maps are keyed by AST nodes no cache holds and would grow by
		// one program per request. Under precompilation the include cache owns
		// the nodes and the maps are bounded by the tree it caches, which is
		// the reuse the pool exists for.
		clear(rt.compiled)
		clear(rt.concatParts)
		clear(rt.sourceSpans)
	}
	// Detached from the request it served, so an idle runtime keeps no
	// connection alive; the next session installs its own.
	rt.SetContext(context.Background())
	p.mu.Lock()
	if len(p.free) < p.size {
		p.free = append(p.free, rt)
	}
	p.mu.Unlock()
}
