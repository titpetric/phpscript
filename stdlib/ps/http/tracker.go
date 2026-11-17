package http

import (
	"io"
	nethttp "net/http"
	"sync"

	"github.com/titpetric/phpscript/runner"
)

// trackers is the free list the per-request answer trackers come from.
var trackers = sync.Pool{New: func() any { return &answerTracker{} }}

// answerTracker records whether a handler began its response and holds its throw.
type answerTracker struct {
	nethttp.ResponseWriter
	wrote   bool
	failure error

	callable *runner.Callable
	request  *nethttp.Request
	args     [2]any

	requestContext runner.Context
	headersApplied bool
}

// Run answers the request the tracker holds, on the worker's runtime.
func (a *answerTracker) Run(rt *runner.Runtime) {
	// The response for the length of the call and a handler that echoes
	// reaches the client and not the process's own output. The tracker and
	// not the raw writer and an echo counts as the response beginning: the
	// 500 guard in answer reads wrote, and a handler that echoed half a
	// document and then threw has already sent its status. The tracker
	// forwards WriteString and an echo still reaches the io.StringWriter
	// net/http implements and pays no clone on the way.
	rt.PushOutput(a)
	defer rt.PopOutput()

	// The request functions (header, $_GET and the rest) resolve this Context
	// at call time; it returns to the worker after the response. Register runs
	// ahead of BeginRequest, which records the request's own connection over
	// the one Register leaves behind.
	request := runner.FromRequest(a.request)
	saved := rt.Context()
	request.Register(rt)
	defer rt.SetContext(saved)
	defer request.Cleanup()
	a.requestContext = request
	defer func() { a.requestContext = runner.Context{} }()

	// The request's own connection and connection_aborted() reports the
	// client that is waiting and a disconnect ends the handler unless it
	// ignored that. The Begin and End pair in place of EnterRequest's
	// restore closure, which is a heap allocation per request.
	defer rt.EndRequest(rt.BeginRequest(a.request.Context()))

	if _, err := a.callable.Invoke(rt, a.args[:]...); err != nil {
		rt.RecordError(err)
		a.failure = err
	}
	a.begin()
	if status := a.requestContext.ResponseStatus(); status != 0 && !a.wrote {
		a.wrote = true
		a.ResponseWriter.WriteHeader(status)
	}
}

// release clears everything the request owns and returns the tracker to the pool.
func (a *answerTracker) release() {
	a.ResponseWriter, a.failure = nil, nil
	a.callable, a.request = nil, nil
	a.args[0], a.args[1] = nil, nil
	a.requestContext, a.headersApplied = runner.Context{}, false
	trackers.Put(a)
}

// begin copies the headers a script staged before the first write goes out.
func (a *answerTracker) begin() {
	if a.headersApplied {
		return
	}
	a.headersApplied = true
	for name, values := range a.requestContext.ResponseHeaders() {
		for _, value := range values {
			a.ResponseWriter.Header().Add(name, value)
		}
	}
}

func (a *answerTracker) WriteHeader(status int) {
	a.begin()
	a.wrote = true
	a.ResponseWriter.WriteHeader(status)
}

func (a *answerTracker) Write(p []byte) (int, error) {
	a.begin()
	if !a.wrote {
		a.wrote = true
		if status := a.requestContext.ResponseStatus(); status != 0 {
			a.ResponseWriter.WriteHeader(status)
		}
	}
	return a.ResponseWriter.Write(p)
}

// WriteString keeps net/http's io.StringWriter reachable through the wrapper.
func (a *answerTracker) WriteString(s string) (int, error) {
	a.begin()
	if !a.wrote {
		a.wrote = true
		if status := a.requestContext.ResponseStatus(); status != 0 {
			a.ResponseWriter.WriteHeader(status)
		}
	}
	if sw, ok := a.ResponseWriter.(io.StringWriter); ok {
		return sw.WriteString(s)
	}
	return a.ResponseWriter.Write([]byte(s))
}

// Unwrap exposes the real writer to net/http's ResponseController.
func (a *answerTracker) Unwrap() nethttp.ResponseWriter { return a.ResponseWriter }
