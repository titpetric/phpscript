package http

import (
	"fmt"
	nethttp "net/http"
	"sync"

	"github.com/titpetric/phpscript/runner"
)

// Mux is net/http's ServeMux with PHP callables as its handlers, each answered
// on a worker runtime. It stays a facade over a ServeMux. No code turns a PHP
// callable into the func(ResponseWriter, *Request) HandleFunc declares.
// docs/use-cases/http-server.md is the surface.
type Mux struct {
	mux *nethttp.ServeMux
	rt  *runner.Runtime

	// pool is the workers handlers run on, shared by all routes.
	//
	// HTTP\Server::listen installs one sized from its $workers and $queue. A mux
	// a Go host mounts and drives through ServeHTTP builds a default one on its
	// first request. Either way the Once builds it once: both paths can reach
	// it. Every request reads it.
	once sync.Once
	pool *runner.Pool

	// root is the mux a Group hangs off, or the mux itself. A group shares its
	// root's ServeMux, pool and mount; only the middleware stack belongs to
	// the group.
	root *Mux

	// middleware wraps the handlers registered after Use, outermost first.
	middleware []*runner.Callable

	// mount, when set, takes every registration in place of the ServeMux. A
	// mux built by NewMountedMux keeps no routes itself. It cannot list them.
	mount func(pattern string, handler nethttp.Handler) error
}

// NewMux returns a router holding no routes.
func NewMux(rt *runner.Runtime) *Mux {
	m := &Mux{mux: nethttp.NewServeMux(), rt: rt}
	m.root = m
	return m
}

// NewMountedMux returns a mux whose routes go to mount and nowhere else.
func NewMountedMux(rt *runner.Runtime, mount func(pattern string, handler nethttp.Handler) error) *Mux {
	m := NewMux(rt)
	m.mount = mount
	return m
}

// Handle registers $handler for $pattern, which is ServeMux.HandleFunc's
// pattern, with {name} segments the handler reads through $r->path_value($name).
//
// $handler is a callable: a closure, a method read from its receiver, spelled
// `$this->fnName`, or the name of a declared function. The mux calls it with the
// response writer and the request. Its echo reaches the response. A throw
// becomes a 500. A Go handler another binding returned, HTTP\FileServer's among
// them, registers unchanged and answers no PHP at all.
// docs/use-cases/http-server.md is the contract.
func (m *Mux) Handle(pattern string, handler any) error {
	if pattern == "" {
		return fmt.Errorf("HTTP\\Mux::handle: pattern is required")
	}
	if given, ok := handler.(nethttp.Handler); ok {
		return m.handleGo(pattern, given)
	}
	// Resolved here: a handler that cannot run on another runtime is an error
	// where the script writes the route, ahead of a 500 on the first request.
	// The handler resolves first: a middleware returns a closure that is
	// callable whatever it holds. Resolving the wrapper alone would pass a
	// broken handler and fail only at request time.
	callable, err := m.rt.AsCallable(handler)
	if err != nil {
		return fmt.Errorf("HTTP\\Mux::handle: %q: %w", pattern, err)
	}
	if len(m.middleware) > 0 {
		wrapped, err := m.wrap(handler)
		if err != nil {
			return fmt.Errorf("HTTP\\Mux::handle: %q: %w", pattern, err)
		}
		if callable, err = m.rt.AsCallable(wrapped); err != nil {
			return fmt.Errorf("HTTP\\Mux::handle: %q: %w", pattern, err)
		}
	}

	serve := nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		// A client already gone gets no handler at all. No write can reach
		// it. Skipping the work is the purpose of the check.
		if r.Context().Err() != nil {
			return
		}
		m.root.answer(callable, w, r)
	})
	return m.register(pattern, serve)
}

func (m *Mux) handleGo(pattern string, handler nethttp.Handler) error {
	if len(m.middleware) > 0 {
		return fmt.Errorf("HTTP\\Mux::handle: %q: a Go handler takes no PHP middleware", pattern)
	}
	return m.register(pattern, handler)
}

func (m *Mux) register(pattern string, handler nethttp.Handler) error {
	if m.root.mount != nil {
		if err := m.root.mount(pattern, handler); err != nil {
			return fmt.Errorf("HTTP\\Mux::handle: %w", err)
		}
		return nil
	}
	m.root.mux.Handle(pattern, handler)
	return nil
}

// Use wraps the handlers registered after it with $middleware, chi's shape.
func (m *Mux) Use(middleware any) error {
	callable, err := m.rt.AsCallable(middleware)
	if err != nil {
		return fmt.Errorf("HTTP\\Mux::use: %w", err)
	}
	m.middleware = append(m.middleware[:len(m.middleware):len(m.middleware)], callable)
	return nil
}

// Group calls $fn with a mux that shares this one's routes and starts with its
// middleware. What $fn passes to Use stays inside the group, keeping a module's
// routes clear of the rest of the router.
func (m *Mux) Group(fn any) error {
	callable, err := m.rt.AsCallable(fn)
	if err != nil {
		return fmt.Errorf("HTTP\\Mux::group: %w", err)
	}
	group := &Mux{rt: m.rt, root: m.root, middleware: m.middleware[:len(m.middleware):len(m.middleware)]}
	if _, err := callable.Invoke(m.rt, group); err != nil {
		return err
	}
	return nil
}

// wrap applies the middleware stack to handler, innermost last, on the registering runtime.
func (m *Mux) wrap(handler any) (any, error) {
	for i := len(m.middleware) - 1; i >= 0; i-- {
		wrapped, err := m.middleware[i].Invoke(m.rt, handler)
		if err != nil {
			return nil, err
		}
		handler = wrapped
	}
	return handler, nil
}

// answer submits one request to a worker and waits for it.
func (m *Mux) answer(callable *runner.Callable, w nethttp.ResponseWriter, r *nethttp.Request) {
	// The tracker is both the work and the wrapper: it lives for the run's
	// length, and submitting it submits no closure. The worker calls its Run
	// method; tracker.go holds it.
	answered := trackers.Get().(*answerTracker)
	answered.ResponseWriter, answered.wrote, answered.failure = w, false, nil
	answered.callable, answered.request = callable, r
	answered.args[0], answered.args[1] = answered, r

	ran := m.workers().Submit(r.Context(), answered)

	if !ran {
		// The client left with the run queued or in flight. A write now
		// would reach no client.
		//
		// The tracker is not put back. A started run notices the disconnect
		// itself. The worker may still be inside the handler, still holding
		// this value; returning it here is the only way a free-list entry
		// could outlive its request and reach a second one. Dropping it costs
		// an allocation on a path the client has left.
		return
	}
	// Only when nothing has gone out yet. A handler that wrote half a document
	// and then threw has already sent its status, and adding another appends
	// "Internal Server Error" to the half document and logs a superfluous
	// WriteHeader; the truncated body is the honest signal.
	if answered.failure != nil && !answered.wrote {
		nethttp.Error(w, nethttp.StatusText(nethttp.StatusInternalServerError), nethttp.StatusInternalServerError)
	}
	// Release last. The code above reads no cleared field, and an idle
	// free-list entry holds no connection alive.
	answered.release()
}

// workers answers the mux's pool, building a default one lazily when no listen call set it.
func (m *Mux) workers() *runner.Pool {
	m = m.root
	m.once.Do(func() { m.pool = runner.NewPool(m.rt, 0, 0, nil) })
	return m.pool
}

// usePool installs the pool handlers run on unless a request already built one.
func (m *Mux) usePool(pool *runner.Pool) {
	m = m.root
	m.once.Do(func() { m.pool = pool })
}

// UsePoolForTest exposes usePool to a test.
func (m *Mux) UsePoolForTest(pool *runner.Pool) { m.usePool(pool) }

// closeWorkers stops the workers once they answer the queue, on shutdown.
func (m *Mux) closeWorkers() {
	m = m.root
	if m.pool != nil {
		m.pool.Close()
	}
}

// ServeHTTP answers one request, the script listening on nothing.
func (m *Mux) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	m.root.mux.ServeHTTP(w, r)
}
