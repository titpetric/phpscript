package http

import (
	"io"
	nethttp "net/http"
	"sync"

	"github.com/titpetric/phpscript/runner"
)

// trackers is the free list the per-request answer trackers come from.
//
// A tracker's lifetime ends when the request does, so this is a
// free list and not a cache: a request takes one, the worker answers through it,
// and answer puts it back having dropped the writer it wrapped. Nothing outside
// one request ever holds a reference, and the one path where that is not certain
// does not put it back; see answer.
var trackers = sync.Pool{New: func() any { return &answerTracker{} }}

// answerTracker records whether a handler has begun its response, so a throw
// afterwards does not try to replace a status that is already on the wire, and
// carries the throw itself back out of the worker.
//
// It forwards and buffers nothing: a handler streaming a large body does not
// have it held in memory for the sake of an error that may never come.
//
// failure is here and not beside the call because the value already lives for
// the length of the request: a local error written from inside the run is a
// second heap cell for the same request, and the code that reads it is the code
// that wrote it.
//
// callable, request and args are the work, so the value submitted to the pool
// is this one and the submit carries no closure. args is an array and no slice
// so the Invoke arguments travel in the tracker: a variadic call site builds
// one slice per request, and a slice of the array builds none.
type answerTracker struct {
	nethttp.ResponseWriter
	wrote   bool
	failure error

	callable *runner.Callable
	request  *nethttp.Request
	args     [2]any

	// done is the channel the pool signals this run's completion through,
	// built once per tracker and lent to every run it serves. See Done.
	done chan struct{}
}

// Done lends the pool this tracker's completion channel, which makes the
// tracker a runner.DoneJob and the submit allocate none.
//
// One buffered slot, emptied by the receive in answer, so the next run the
// tracker serves signals through the same channel. The safety rule the lending
// carries is the free list's own: a tracker whose run answer did not see
// finish never returns here, so no channel a worker may still signal through
// reaches a second request.
func (a *answerTracker) Done() chan struct{} {
	if a.done == nil {
		a.done = make(chan struct{}, 1)
	}
	return a.done
}

// Run answers the request this tracker holds, on the worker's runtime. It is
// the runner.Job the pool runs, which is this value and not a closure over it.
func (a *answerTracker) Run(rt *runner.Runtime) {
	// The response for the length of the call, so a handler that echoes
	// reaches the client and not the process's own output. The tracker and
	// not the raw writer, so an echo counts as the response beginning: the
	// 500 guard in answer reads wrote, and a handler that echoed half a
	// document and then threw has already sent its status. The tracker
	// forwards WriteString, so an echo still reaches the io.StringWriter
	// net/http implements and pays no clone on the way.
	rt.PushOutput(a)
	defer rt.PopOutput()

	// The connection being answered, so connection_aborted() reports the
	// client that is waiting and a disconnect ends the handler unless it
	// ignored that. The Begin and End pair in place of EnterRequest's
	// restore closure, which is a heap allocation per request.
	defer rt.EndRequest(rt.BeginRequest(a.request.Context()))

	if _, err := a.callable.Invoke(rt, a.args[:]...); err != nil {
		rt.RecordError(err)
		a.failure = err
	}
}

// release clears everything the request owns and puts the tracker back. The
// writer, the request and the boxed arguments all reach outside this package,
// so a tracker left holding them would keep a connection and a request body
// alive for as long as the free list does.
func (a *answerTracker) release() {
	a.ResponseWriter, a.failure = nil, nil
	a.callable, a.request = nil, nil
	a.args[0], a.args[1] = nil, nil
	trackers.Put(a)
}

func (a *answerTracker) WriteHeader(status int) {
	a.wrote = true
	a.ResponseWriter.WriteHeader(status)
}

func (a *answerTracker) Write(p []byte) (int, error) {
	a.wrote = true
	return a.ResponseWriter.Write(p)
}

// WriteString keeps the io.StringWriter net/http's response writer implements
// reachable through the wrapper. io.WriteString finds the method where the
// writer underneath has one, and a wrapper hiding it sends every such write,
// fwrite($w, ...) among them, down the fallback that clones the string onto
// the heap. A writer without the method gets the same fallback here.
func (a *answerTracker) WriteString(s string) (int, error) {
	a.wrote = true
	if sw, ok := a.ResponseWriter.(io.StringWriter); ok {
		return sw.WriteString(s)
	}
	return a.ResponseWriter.Write([]byte(s))
}

// Unwrap exposes the real writer to net/http's ResponseController, so a handler
// reaching for Flush or a deadline still finds it.
func (a *answerTracker) Unwrap() nethttp.ResponseWriter { return a.ResponseWriter }
