package http

import (
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
)

// stringWriterRecorder counts which write method a caller reaches, so a test
// can tell a forwarded WriteString from the []byte fallback.
type stringWriterRecorder struct {
	*httptest.ResponseRecorder
	stringWrites int
}

func (r *stringWriterRecorder) WriteString(s string) (int, error) {
	r.stringWrites++
	return r.ResponseRecorder.WriteString(s)
}

// TestTrackerForwardsWriteString holds the tracker to the writer it wraps: a
// writer with WriteString gets the string, one without gets the bytes, and
// either way the tracker records that the response has begun.
func TestTrackerForwardsWriteString(t *testing.T) {
	t.Parallel()

	t.Run("forwards to a StringWriter", func(t *testing.T) {
		t.Parallel()
		inner := &stringWriterRecorder{ResponseRecorder: httptest.NewRecorder()}
		tracker := &answerTracker{ResponseWriter: inner}

		n, err := io.WriteString(tracker, "answer")
		if err != nil || n != len("answer") {
			t.Fatalf("WriteString = %d, %v, want %d, nil", n, err, len("answer"))
		}
		if inner.stringWrites != 1 {
			t.Errorf("stringWrites = %d, want 1: the write took the fallback", inner.stringWrites)
		}
		if !tracker.wrote {
			t.Error("wrote = false, want true: a throw after this write would answer a second status")
		}
		if got := inner.Body.String(); got != "answer" {
			t.Errorf("body = %q, want %q", got, "answer")
		}
	})

	t.Run("falls back to Write", func(t *testing.T) {
		t.Parallel()
		inner := httptest.NewRecorder()
		var bare nethttp.ResponseWriter = headerOnlyWriter{inner}
		tracker := &answerTracker{ResponseWriter: bare}

		n, err := io.WriteString(tracker, "answer")
		if err != nil || n != len("answer") {
			t.Fatalf("WriteString = %d, %v, want %d, nil", n, err, len("answer"))
		}
		if !tracker.wrote {
			t.Error("wrote = false, want true")
		}
		if got := inner.Body.String(); got != "answer" {
			t.Errorf("body = %q, want %q", got, "answer")
		}
	})
}

// headerOnlyWriter narrows a recorder to the three ResponseWriter methods, so
// the tracker sees a writer with no WriteString of its own.
type headerOnlyWriter struct {
	inner *httptest.ResponseRecorder
}

func (w headerOnlyWriter) Header() nethttp.Header      { return w.inner.Header() }
func (w headerOnlyWriter) WriteHeader(status int)      { w.inner.WriteHeader(status) }
func (w headerOnlyWriter) Write(p []byte) (int, error) { return w.inner.Write(p) }
