package runner

import (
	"context"
	"io"
	"testing"
)

// TestRuntime_BeginRequest covers the save and restore pair the mux defers
// around a handler: BeginRequest records the connection, clears the abort
// choice and marks the request live, and EndRequest puts every saved field
// back, nesting included.
func TestRuntime_BeginRequest(t *testing.T) {
	t.Parallel()
	rt := New(io.Discard, Options{})
	rt.SetIgnoreUserAbort(true)

	outer, cancelOuter := context.WithCancel(context.Background())
	defer cancelOuter()
	saved := rt.BeginRequest(outer)

	if !rt.inRequest {
		t.Error("inRequest = false inside a request")
	}
	if rt.IgnoreUserAbort() {
		t.Error("a new request starts with the abort observed")
	}
	if rt.ClientContext() != outer {
		t.Error("ClientContext() is not the request entered")
	}

	inner, cancelInner := context.WithCancel(context.Background())
	defer cancelInner()
	var nested RequestState = rt.BeginRequest(inner)
	if rt.ClientContext() != inner {
		t.Error("ClientContext() is not the nested request")
	}
	rt.EndRequest(nested)
	if rt.ClientContext() != outer {
		t.Error("EndRequest does not restore the outer request")
	}

	rt.EndRequest(saved)
	if rt.inRequest {
		t.Error("inRequest = true after the last EndRequest")
	}
	if !rt.IgnoreUserAbort() {
		t.Error("EndRequest does not restore the abort choice")
	}
}
