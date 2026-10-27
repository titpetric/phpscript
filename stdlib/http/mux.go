package http

import (
	"fmt"
	nethttp "net/http"
	"sync"

	"github.com/titpetric/phpscript/runner"
)

// Mux is net/http's ServeMux with PHP callables as its handlers, answered on a
// worker runtime each. It stays a facade and no ServeMux itself because nothing
// turns a PHP callable into the func(ResponseWriter, *Request) that HandleFunc
// declares. docs/use-cases/http-server.md is the surface.
type Mux struct {
	mux *nethttp.ServeMux
	rt  *runner.Runtime

	// pool is the workers handlers run on, shared by every route.
	//
	// HTTP\Server::listen installs one sized from its $workers and $queue; a mux
	// a Go host mounts and drives through ServeHTTP itself builds a default one
	// on its first request. Either way the Once builds it once: the two paths
	// can both reach it, and every request reads it.
	once sync.Once
	pool *runner.Pool
}

// NewMux returns a router with no routes on it.
func NewMux(rt *runner.Runtime) *Mux {
	return &Mux{mux: nethttp.NewServeMux(), rt: rt}
}

// Handle registers $handler for $pattern, which is ServeMux.HandleFunc's
// pattern, with {name} segments the handler reads through $r->path_value($name).
//
// $handler is a callable: a closure, a method read off its receiver as
// $this->fnName, or the name of a declared function. The mux calls it with the
// response writer and the request, what it echoes reaches the response, and a
// throw becomes a 500. docs/use-cases/http-server.md is the contract.
func (m *Mux) Handle(pattern string, handler any) error {
	if pattern == "" {
		return fmt.Errorf("HTTP\\Mux::handle: pattern is required")
	}
	// Resolved here and a handler that cannot run on another runtime is an
	// error where the script writes the route, ahead of a 500 on the first
	// request that reaches it.
	callable, err := m.rt.AsCallable(handler)
	if err != nil {
		return fmt.Errorf("HTTP\\Mux::handle: %q: %w", pattern, err)
	}

	m.mux.HandleFunc(pattern, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		// A client that has already gone gets no handler at all. No write
		// now reaches it, and skipping the work is what the check
		// is for.
		if r.Context().Err() != nil {
			return
		}
		m.answer(callable, w, r)
	})
	return nil
}

// answer submits one request to a worker and waits for it.
func (m *Mux) answer(callable *runner.Callable, w nethttp.ResponseWriter, r *nethttp.Request) {
	// The tracker is the work as well as the wrapper: it lives for the length
	// of the run already and submitting it submits no closure. Its Run method
	// is what the worker calls, and tracker.go holds it.
	answered := trackers.Get().(*answerTracker)
	answered.ResponseWriter, answered.wrote, answered.failure = w, false, nil
	answered.callable, answered.request = callable, r
	answered.args[0], answered.args[1] = answered, r

	ran := m.workers().Submit(r.Context(), answered)

	if !ran {
		// The client left with the run queued or in flight. A write now
		// would reach no client.
		//
		// The tracker is not put back. A run already started notices the
		// disconnect itself and the worker may still be inside the handler and
		// still holding this value; returning it here is the one way a free-list
		// entry could outlive its request and reach a second one. Dropping it
		// costs an allocation on a path a client has already left.
		return
	}
	// Only when nothing has gone out yet. A handler that wrote half a document
	// and then threw has already sent its status, and adding another appends
	// "Internal Server Error" to the half document and logs a superfluous
	// WriteHeader; the truncated body is the honest signal.
	if answered.failure != nil && !answered.wrote {
		nethttp.Error(w, nethttp.StatusText(nethttp.StatusInternalServerError), nethttp.StatusInternalServerError)
	}
	// The release last and nothing above reads a cleared field and no idle
	// free-list entry holds a connection alive.
	answered.release()
}

// workers answers the pool this mux runs on, building a default one for a mux
// that never called listen(): a Go host that mounted it serves it concurrently
// too, and the script runs on the runtime that built it.
func (m *Mux) workers() *runner.Pool {
	m.once.Do(func() { m.pool = runner.NewPool(m.rt, 0, 0, nil) })
	return m.pool
}

// usePool installs the workers handlers run on, sized by the server that is
// about to serve them. It loses to a pool a request already built, which cannot
// happen through HTTP\Server: the script calls listen() before anything can
// arrive.
func (m *Mux) usePool(pool *runner.Pool) {
	m.once.Do(func() { m.pool = pool })
}

// UsePoolForTest is usePool under a name a test can reach.
func (m *Mux) UsePoolForTest(pool *runner.Pool) { m.usePool(pool) }

// closeWorkers stops them once the workers have answered the queue, for a
// server shutting down.
func (m *Mux) closeWorkers() {
	if m.pool != nil {
		m.pool.Close()
	}
}

// ServeHTTP answers one request, the script listening on nothing.
func (m *Mux) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	m.mux.ServeHTTP(w, r)
}
