package http

import (
	"fmt"
	nethttp "net/http"

	"github.com/titpetric/phpscript/runner"
)

// Mux routes requests to the PHP functions that answer them. It is net/http's
// ServeMux and nothing more: routing is all it does, and serving is HTTP\Server.
//
// It is a facade over *net/http.ServeMux rather than the value itself, which is
// the exception to how this package binds net/http. Registering the ServeMux
// directly gets as far as `$mux->handle_func("GET /x", function ($w, $r) {...})`
// and no further: the argument bridge coerces values, and a PHP closure is a
// func(...any) (any, error) that nothing turns into the func(ResponseWriter,
// *Request) the parameter declares. It answers "Argument #2 must be of type
// callable, Closure given".
//
// Teaching the bridge that conversion would reach past this one binding, and it
// would be a trap: it has no way to know whether the Go side calls the callback
// on the caller's goroutine, as usort does, or holds it and calls it on its
// own, as a router does. A Runtime serves one goroutine, so the difference is
// between working and a fatal concurrent map write. Handle knows the answer for
// a router and the bridge would have to guess, which is what the facade is for.
//
// The pattern syntax is net/http's own, "GET /users/{id}" and the rest, so what
// a script writes is what the Go router reads.
type Mux struct {
	mux *nethttp.ServeMux
	rt  *runner.Runtime
}

// NewMux returns a router with no routes on it.
func NewMux(rt *runner.Runtime) *Mux {
	return &Mux{mux: nethttp.NewServeMux(), rt: rt}
}

// Handle registers $handler for $pattern, which is a net/http pattern: a bare
// path, or a method and a path, with {name} segments the handler reads back
// through $r->path_value($name).
//
// The handler is called with the response writer and the request, the two
// net/http values themselves, so it answers through $w->write($body) and
// $w->header()->set($name, $value) rather than by echoing. Output a handler
// echoes goes where the runtime's output goes, which is not the response.
//
// A handler that throws is one request's problem: it is reported and answered
// with a 500, and the server goes on. Unless the host installed an error
// handler with Runtime.OnError, which means "report it and carry on from the
// next statement" for every PHP error: the throw is then reported there, the
// handler runs to its end, and the request is answered with whatever it had
// written by then.
func (m *Mux) Handle(pattern string, handler any) error {
	if pattern == "" {
		return fmt.Errorf("HTTP\\Mux::handle: pattern is required")
	}
	call, ok := m.rt.Callable(handler)
	if !ok {
		return fmt.Errorf("HTTP\\Mux::handle: %q is not a valid callback", pattern)
	}

	m.mux.HandleFunc(pattern, func(w nethttp.ResponseWriter, r *nethttp.Request) {
		// A client that has already gone gets no handler at all. Nothing
		// written now reaches it, and skipping the work is the whole point of
		// noticing; a handler could not opt out of this with
		// ignore_user_abort anyway, because it would be stopped before its
		// first statement ran.
		if r.Context().Err() != nil {
			return
		}

		// The runtime, not this mux: net/http answers each request on its own
		// goroutine, a Runtime interprets one program at a time, and the script
		// that built this mux is a third caller. A mutex of its own would leave
		// two muxes on one runtime interpreting at once, and would never hold
		// the script itself out.
		defer m.rt.LockExec()()
		// The connection this handler is answering, so connection_aborted()
		// reports the client that is waiting rather than whatever started the
		// script, and a disconnect ends the handler unless it ignored that.
		defer m.rt.EnterRequest(r.Context())()

		answered := &answerTracker{ResponseWriter: w}
		if _, err := call(answered, r); err != nil {
			// The handler threw. The script is still serving, so this is the
			// one request's problem: it is reported to the runtime's error sink
			// and answered with a status, not taken to the process.
			m.rt.RecordError(err)
			// Only when nothing has gone out yet. A handler that wrote half a
			// document and then threw has already sent its status, and adding
			// another appends "Internal Server Error" to the half document and
			// logs a superfluous WriteHeader; the truncated body is the honest
			// signal, and the error is on the runtime's sink either way.
			if !answered.wrote {
				nethttp.Error(w, nethttp.StatusText(nethttp.StatusInternalServerError), nethttp.StatusInternalServerError)
			}
		}
	})
	return nil
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
