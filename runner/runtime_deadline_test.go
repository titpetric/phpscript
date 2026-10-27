package runner

import (
	"context"
	"io"
	"testing"
	"time"
)

// TestRequestScopesOnlyItsConnection holds what a request owns and what it does
// not. The connection and the abort policy go back when the handler leaves;
// the time limit is the runtime's and a handler moving it means to move it,
// which is php's rule where the request is the script.
func TestRequestScopesOnlyItsConnection(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})
	rt.SetContext(context.Background())
	rt.SetTimeLimit(time.Hour)

	parked := rt.Context()
	gone, disconnect := context.WithCancel(context.Background())
	leave := rt.EnterRequest(gone)

	rt.SetIgnoreUserAbort(true)
	rt.SetTimeLimit(2 * time.Hour)
	disconnect()
	if !rt.ConnectionAborted() {
		t.Error("the handler did not see its own client leave")
	}

	leave()

	if rt.ConnectionAborted() {
		t.Error("the request's disconnect was left on the script")
	}
	if rt.IgnoreUserAbort() {
		t.Error("the handler's abort policy was left on the script")
	}
	if rt.TimeLimit() != 2*time.Hour {
		t.Errorf("limit = %s, want the handler's 2h to have moved the runtime's", rt.TimeLimit())
	}
	if rt.Context() != parked {
		t.Error("the context a blocked binding holds was replaced")
	}
	if err := parked.Err(); err != nil {
		t.Errorf("the context a blocked binding holds was cancelled: %v", err)
	}
}

// TestRuntime_ClientContext covers which of the two contexts a binding waits
// on. Inside a handler the request's and a call that should end when the caller
// hangs up does; outside one there is no request and the script's own serves,
// and the same binding still has something to wait on. Leaving the handler puts
// the previous one back and one request's disconnect stays off the next.
func TestRuntime_ClientContext(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})
	host, hangUp := context.WithCancel(context.Background())
	rt.SetContext(host)

	if got := rt.ClientContext(); got != host {
		t.Error("outside a handler the client context is not the one the host gave")
	}

	gone, disconnect := context.WithCancel(context.Background())
	leave := rt.EnterRequest(gone)
	if got := rt.ClientContext(); got != gone {
		t.Error("inside a handler the client context is not the request's")
	}

	disconnect()
	if err := rt.ClientContext().Err(); err == nil {
		t.Error("the client context did not end with the connection")
	}
	// The disconnect leaves the script's own context alone and a serving
	// script answers the next request after one caller hangs up.
	if err := rt.Context().Err(); err != nil {
		t.Errorf("the disconnect ended the script's context: %v", err)
	}

	leave()
	if got := rt.ClientContext(); got != host {
		t.Error("the request's context was left behind as the client context")
	}
	if err := rt.ClientContext().Err(); err != nil {
		t.Errorf("the request's disconnect was left on the client context: %v", err)
	}

	// The host ending is the connection ending for a script outside a handler,
	// with no other server behind it.
	hangUp()
	if err := rt.ClientContext().Err(); err == nil {
		t.Error("the host ending did not reach the client context")
	}

	// With no connection recorded at all the script's own context is the
	// answer and a binding always has something to wait on.
	rt.client = nil
	if got := rt.ClientContext(); got != rt.Context() {
		t.Error("with no connection the client context is not the script's")
	}
}

// TestScriptLimitCancelsTheBindingContext is the other side: the script's own
// limit does end a parked context, because that is how a serving script stops.
func TestScriptLimitCancelsTheBindingContext(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})
	rt.SetContext(context.Background())

	parked := rt.Context()
	rt.SetTimeLimit(50 * time.Millisecond)

	select {
	case <-parked.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("the limit did not end the context a binding was parked on")
	}
	if err := rt.checkDeadline(); err == nil {
		t.Error("the limit fired but the next statement was not stopped")
	}
}

// TestReleaseDeadlineStopsTheTimers keeps a limit from outliving the program
// that set it: an armed timer on a reused runtime is a program's deadline
// applied to the next one.
func TestReleaseDeadlineStopsTheTimers(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})
	rt.SetContext(context.Background())
	rt.SetTimeLimit(time.Hour)
	if rt.scriptTimer == nil {
		t.Fatal("no timer was armed")
	}

	rt.resetLimits()
	if rt.scriptTimer != nil {
		t.Error("the timer was left armed")
	}
	if rt.TimeLimit() != 0 {
		t.Errorf("limit = %s, want it cleared", rt.TimeLimit())
	}
}
