package http

import (
	"context"
	nethttp "net/http"
	"net/http/httptest"
	"testing"
)

// TestAborted covers a nil request, a live one and one whose client has gone.
func TestAborted(t *testing.T) {
	if aborted(nil) {
		t.Error("a nil request reported aborted")
	}
	live := httptest.NewRequest(nethttp.MethodGet, "/", nil)
	if aborted(live) {
		t.Error("a live request reported aborted")
	}
	ctx, cancel := context.WithCancel(live.Context())
	cancel()
	if !aborted(live.WithContext(ctx)) {
		t.Error("a cancelled request reported live")
	}
}
