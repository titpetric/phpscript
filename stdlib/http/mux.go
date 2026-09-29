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
// A handler is named rather than written inline, because a request is answered
// on a runtime of its own and a name is the only part of a callable that
// travels. See Handle.
//
// It is a facade over *net/http.ServeMux rather than the value itself, which is
// the exception to how this package binds net/http. Registering the ServeMux
// directly gets as far as `$mux->handle_func("GET /x", ...)` and no further:
// the argument bridge coerces values, and nothing turns a PHP callable into the
// func(ResponseWriter, *Request) the parameter declares.
type Mux struct {
	mux *nethttp.ServeMux
	rt  *runner.Runtime

	// pool is the forks handlers run on, shared by every route.
	//
	// HTTP\Server::listen installs one sized from its $workers; a mux a Go host
	// mounts and drives through ServeHTTP itself builds one on its first
	// request. Either way it is built once, which is what the Once is for: the
	// two paths can both reach it, and every request reads it.
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
// $handler is a closure, or the name of a declared function. Either way it is a
// program counter: a request is answered on a runtime of its own, and what
// crosses is the declaration, with everything the call needs arriving in its
// arguments.
//
// A closure that captures - `use (...)`, or the $this a closure written inside
// a method binds - is refused, because the captured scope belongs to the
// runtime that built it and two requests would be sharing it. A handler takes
// its state from $w and $r.
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
	callback, err := m.rt.AsCallback(handler)
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
		m.answer(callback, w, r)
	})
	return nil
}

// answer runs one request on a runtime of its own.
func (m *Mux) answer(callback runner.Callback, w nethttp.ResponseWriter, r *nethttp.Request) {
	rt, ok := m.checkout(r)
	if !ok {
		// Every fork is busy and the client left while queued. There is nobody
		// left to answer.
		return
	}
	defer m.checkin(rt)

	// The response for the length of the call, so a handler that echoes reaches
	// the client rather than the process's own output.
	rt.PushOutput(w)
	defer rt.PopOutput()

	// The connection being answered, so connection_aborted() reports the client
	// that is waiting and a disconnect ends the handler unless it ignored that.
	defer rt.EnterRequest(r.Context())()

	answered := &answerTracker{ResponseWriter: w}
	if _, err := callback.Invoke(rt, answered, r); err != nil {
		rt.RecordError(err)
		// Only when nothing has gone out yet. A handler that wrote half a
		// document and then threw has already sent its status, and adding
		// another appends "Internal Server Error" to the half document and logs
		// a superfluous WriteHeader; the truncated body is the honest signal.
		if !answered.wrote {
			nethttp.Error(w, nethttp.StatusText(nethttp.StatusInternalServerError), nethttp.StatusInternalServerError)
		}
	}
}

// checkout takes the runtime this request runs on: a fork when the mux is
// serving, and the one that built it when a Go host drives ServeHTTP directly.
func (m *Mux) checkout(r *nethttp.Request) (*runner.Runtime, bool) {
	// A mux nothing called listen() on still answers on a runtime of its own:
	// a Go host that mounted it serves it concurrently too, and the runtime
	// that built it is running the script.
	m.once.Do(func() { m.pool = runner.NewPool(m.rt, 0, nil) })
	return m.pool.Get(r.Context().Done())
}

// checkin gives the runtime back.
func (m *Mux) checkin(rt *runner.Runtime) { m.pool.Put(rt) }

// usePool installs the forks handlers run on, sized by the server that is about
// to serve them. It loses to a pool a request already built, which cannot
// happen through HTTP\Server: listen() is called before anything can arrive.
func (m *Mux) usePool(pool *runner.Pool) {
	m.once.Do(func() { m.pool = pool })
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
