package runner

import (
	"context"
	"io"
	"testing"
	"time"
)

// TestRequestScopesOnlyItsConnection holds what a request owns and what it does
// not. The connection and the abort policy are put back when the handler
// leaves; the time limit is the runtime's and a handler moving it means to move
// it, which is php's rule where the request is the script.
func TestRequestScopesOnlyItsConnection(t *testing.T) {
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

// TestScriptLimitCancelsTheBindingContext is the other side: the script's own
// limit does end what is parked, because that is how a serving script stops.
func TestScriptLimitCancelsTheBindingContext(t *testing.T) {
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
