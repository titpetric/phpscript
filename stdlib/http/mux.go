package http

import (
	"fmt"
	nethttp "net/http"
	"sync"

	"github.com/titpetric/phpscript/runner"
)

// Mux routes requests to the PHP functions that answer them. It is net/http's
// ServeMux and nothing more: routing is all it does, and serving is HTTP\Server.
//
// A request is answered on a runtime of its own, from a fixed set of workers
// fed by one queue. See Handle for what a handler is and runner.Pool for what
// bounds them.
//
// It is a facade over *net/http.ServeMux rather than the value itself, which is
// the exception to how this package binds net/http. Registering the ServeMux
// directly gets as far as `$mux->handle_func("GET /x", ...)` and no further:
// the argument bridge coerces values, and nothing turns a PHP callable into the
// func(ResponseWriter, *Request) the parameter declares.
type Mux struct {
	mux *nethttp.ServeMux
	rt  *runner.Runtime

	// pool is the workers handlers run on, shared by every route.
	//
	// HTTP\Server::listen installs one sized from its $workers and $queue; a mux
	// a Go host mounts and drives through ServeHTTP itself builds a default one
	// on its first request. Either way it is built once, which is what the Once
	// is for: the two paths can both reach it, and every request reads it.
	once sync.Once
	pool *runner.Pool
}

// NewMux returns a router with no routes on it.
func NewMux(rt *runner.Runtime) *Mux {
	return &Mux{mux: nethttp.NewServeMux(), rt: rt}
}

// Handle registers $handler for $pattern, which is a net/http pattern: a bare
// path, or a method and a path, with {name} segments the handler reads back
// through $r->path_value($name).
//
// $handler is a callable: a closure, a method read off its receiver as
// $this->fnName, or the name of a declared function. Each is a program counter:
// a request is answered on a runtime of its own, and what crosses is the
// declaration, with everything the call needs arriving in its arguments.
//
// What a handler carries - a closure's `use (...)` values and the $this it
// binds, a bound method's receiver - comes along and is shared by every request
// answering through it, the way a Go handler closing over its configuration is.
// Read it; writing to it from a handler is two requests writing one value. A
// handler's own state arrives in $w and $r.
//
// A handler that calls another callable - middleware wrapping the handler it
// captured, a comparator handed to usort, a closure read off a shared object -
// runs that call on the worker too. The value crossed; the execution did not
// follow it back to the runtime that built it, which is what keeps the wrapped
// call writing to this request's response and off another worker's stack.
//
// The array($object, "method") spelling of a callable is not accepted here. It
// stays a callable everywhere else; docs/README.md records the difference.
//
// The handler is called with the response writer and the request, the two
// net/http values themselves, so it answers through $w->write($body) and
// $w->header()->set($name, $value). What it echoes reaches the response too:
// the runtime answering the request writes there for the length of the call.
//
// A handler that throws is one request's problem: it is reported to the
// runtime's error sink and answered with a 500 if nothing has gone out yet, and
// the server goes on. Unless the host installed Runtime.OnError, which means
// "report it and carry on from the next statement" for every PHP error; the
// handler then runs to its end and the request is answered with whatever it had
// written by then.
func (m *Mux) Handle(pattern string, handler any) error {
	if pattern == "" {
		return fmt.Errorf("HTTP\\Mux::handle: pattern is required")
	}
	// Described here, so a handler that cannot run on another runtime is an
	// error where the route is written rather than a 500 on the first request
	// that reaches it.
	callable, err := m.rt.AsCallable(handler)
	if err != nil {
		return fmt.Errorf("HTTP\\Mux::handle: %q: %w", pattern, err)
	}

	m.mux.HandleFunc(pattern, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		// A client that has already gone gets no handler at all. Nothing
		// written now reaches it, and skipping the work is the whole point of
		// noticing.
		if r.Context().Err() != nil {
			return
		}
		m.answer(callable, w, r)
	})
	return nil
}

// answer submits one request to a worker and waits for it.
func (m *Mux) answer(callable *runner.Callable, w nethttp.ResponseWriter, r *nethttp.Request) {
	answered := &answerTracker{ResponseWriter: w}
	var failure error

	ran := m.workers().Submit(r.Context(), func(rt *runner.Runtime) {
		// The response for the length of the call, so a handler that echoes
		// reaches the client rather than the process's own output.
		rt.PushOutput(w)
		defer rt.PopOutput()

		// The connection being answered, so connection_aborted() reports the
		// client that is waiting and a disconnect ends the handler unless it
		// ignored that.
		defer rt.EnterRequest(r.Context())()

		if _, err := callable.Invoke(rt, answered, r); err != nil {
			rt.RecordError(err)
			failure = err
		}
	})

	if !ran {
		// The client left while the run was queued or in flight. Nothing
		// written now would reach it.
		return
	}
	// Only when nothing has gone out yet. A handler that wrote half a document
	// and then threw has already sent its status, and adding another appends
	// "Internal Server Error" to the half document and logs a superfluous
	// WriteHeader; the truncated body is the honest signal.
	if failure != nil && !answered.wrote {
		nethttp.Error(w, nethttp.StatusText(nethttp.StatusInternalServerError), nethttp.StatusInternalServerError)
	}
}

// workers answers the pool this mux runs on, building a default one for a mux
// nothing called listen() on: a Go host that mounted it serves it concurrently
// too, and the runtime that built it is running the script.
func (m *Mux) workers() *runner.Pool {
	m.once.Do(func() { m.pool = runner.NewPool(m.rt, 0, 0, nil) })
	return m.pool
}

// usePool installs the workers handlers run on, sized by the server that is
// about to serve them. It loses to a pool a request already built, which cannot
// happen through HTTP\Server: listen() is called before anything can arrive.
func (m *Mux) usePool(pool *runner.Pool) {
	m.once.Do(func() { m.pool = pool })
}

// UsePoolForTest installs the workers, for a test sizing them itself. The
// server calls usePool; this is the same thing with a name a test can reach.
func (m *Mux) UsePoolForTest(pool *runner.Pool) { m.usePool(pool) }

// closeWorkers stops them once what is queued has been answered, for a server
// shutting down.
func (m *Mux) closeWorkers() {
	if m.pool != nil {
		m.pool.Close()
	}
}

// ServeHTTP answers one request, so a Go host can mount a script's router
// without the script listening on anything.
func (m *Mux) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	m.mux.ServeHTTP(w, r)
}

// answerTracker records whether a handler has begun its response, so a throw
// afterwards does not try to replace a status that is already on the wire.
//
// It forwards rather than buffers: a handler streaming a large body should not
// have it held in memory for the sake of an error that may never come.
type answerTracker struct {
	nethttp.ResponseWriter
	wrote bool
}

func (a *answerTracker) WriteHeader(status int) {
	a.wrote = true
	a.ResponseWriter.WriteHeader(status)
}

func (a *answerTracker) Write(p []byte) (int, error) {
	a.wrote = true
	return a.ResponseWriter.Write(p)
}

// Unwrap hands the real writer to net/http's ResponseController, so a handler
// reaching for Flush or a deadline still finds it.
func (a *answerTracker) Unwrap() nethttp.ResponseWriter { return a.ResponseWriter }
