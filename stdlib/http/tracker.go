package http

import (
	"io"
	nethttp "net/http"
	"sync"
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
