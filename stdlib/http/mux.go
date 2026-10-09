package http

import (
	"fmt"
	nethttp "net/http"
	"sync"

	"github.com/titpetric/phpscript/runner"
)

// Mux is net/http's ServeMux with PHP callables as its handlers, answered on a
// worker runtime each. It is a facade and no ServeMux itself because
// nothing turns a PHP callable into the func(ResponseWriter, *Request) that
// HandleFunc declares. docs/use-cases/http-server.md is the surface.
type Mux struct {
	mux *nethttp.ServeMux
	rt  *runner.Runtime

	// pool is the workers handlers run on, shared by every route.
	//
	// HTTP\Server::listen installs one sized from its $workers and $queue; a mux
	// a Go host mounts and drives through ServeHTTP itself builds a default one
	// on its first request. Either way it is built once, which the Once
	// is for: the two paths can both reach it, and every request reads it.
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
// $this->fnName, or the name of a declared function. It is called with the
// response writer and the request, what it echoes reaches the response, and a
// throw is answered with a 500. docs/use-cases/http-server.md is the contract.
func (m *Mux) Handle(pattern string, handler any) error {
	if pattern == "" {
		return fmt.Errorf("HTTP\\Mux::handle: pattern is required")
	}
	// Described here, so a handler that cannot run on another runtime is an
	// error where the route is written, ahead of a 500 on the first request
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

// trackers is the free list the per-request answer trackers come from.
//
// A tracker's lifetime ends when the request does, so this is a
// free list and not a cache: a request takes one, the worker answers through it,
// and answer puts it back having dropped the writer it wrapped. Nothing outside
// one request ever holds a reference, and the one path where that is not certain
// does not put it back; see answer.
var trackers = sync.Pool{New: func() any { return &answerTracker{} }}

// answer submits one request to a worker and waits for it.
func (m *Mux) answer(callable *runner.Callable, w nethttp.ResponseWriter, r *nethttp.Request) {
	answered := trackers.Get().(*answerTracker)
	answered.ResponseWriter, answered.wrote, answered.failure = w, false, nil

	ran := m.workers().Submit(r.Context(), func(rt *runner.Runtime) {
		// The response for the length of the call, so a handler that echoes
		// reaches the client and not the process's own output.
		rt.PushOutput(w)
		defer rt.PopOutput()

		// The connection being answered, so connection_aborted() reports the
		// client that is waiting and a disconnect ends the handler unless it
		// ignored that.
		defer rt.EnterRequest(r.Context())()

		if _, err := callable.Invoke(rt, answered, r); err != nil {
			rt.RecordError(err)
			answered.failure = err
		}
	})

	if !ran {
		// The client left while the run was queued or in flight. Nothing written
		// now would reach it.
		//
		// The tracker is not put back. A run already started is left
		// to notice the disconnect itself, so the worker may still be inside the
		// handler and still holding this value; returning it here is the one way a
		// free-list entry could outlive its request and be handed to a second one.
		// Dropping it costs an allocation on a path a client has already left.
		return
	}
	// Only when nothing has gone out yet. A handler that wrote half a document
	// and then threw has already sent its status, and adding another appends
	// "Internal Server Error" to the half document and logs a superfluous
	// WriteHeader; the truncated body is the honest signal.
	if answered.failure != nil && !answered.wrote {
		nethttp.Error(w, nethttp.StatusText(nethttp.StatusInternalServerError), nethttp.StatusInternalServerError)
	}
	// The writer last, so nothing above reads a cleared field and no connection is
	// held alive by an idle free-list entry.
	answered.ResponseWriter, answered.failure = nil, nil
	trackers.Put(answered)
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
// afterwards does not try to replace a status that is already on the wire, and
// carries the throw itself back out of the worker.
//
// It forwards and buffers nothing: a handler streaming a large body does not
// have it held in memory for the sake of an error that may never come.
//
// failure is here and not beside the call because answer's closure captures
// this value already: a local error written from inside the closure is a second
// heap cell for the same request, and the code that reads it is the code that
// wrote it.
type answerTracker struct {
	nethttp.ResponseWriter
	wrote   bool
	failure error
}

func (a *answerTracker) WriteHeader(status int) {
	a.wrote = true
	a.ResponseWriter.WriteHeader(status)
}

func (a *answerTracker) Write(p []byte) (int, error) {
	a.wrote = true
	return a.ResponseWriter.Write(p)
}

// Unwrap exposes the real writer to net/http's ResponseController, so a handler
// reaching for Flush or a deadline still finds it.
func (a *answerTracker) Unwrap() nethttp.ResponseWriter { return a.ResponseWriter }
