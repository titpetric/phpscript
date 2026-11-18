package http

import (
	"context"
	"io"
	nethttp "net/http"
	"net/http/httptest"
	"testing"

	"github.com/titpetric/phpscript/runner"
)

// TestRegister covers the constructors and functions the area installs.
func TestRegister(t *testing.T) {
	rt := runner.New(io.Discard, runner.Options{})
	Register(rt)
	if _, ok := rt.LookupConstructor(Namespace + `\ServeMux`); !ok {
		t.Fatal("Register installed no ServeMux")
	}
	if _, ok := rt.LookupFunc(Namespace + `\Aborted`); !ok {
		t.Fatal("Register installed no Aborted")
	}
}

// TestAborted covers a nil request, a live one and a gone client.
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
