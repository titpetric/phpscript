package annotations

import (
	"context"
	"net/http"
	"testing"
)

// TestModule covers the zero module every annotation module embeds: it
// answers the lifecycle the module leaves alone, and reports the name
// Options.Modules filters on.
func TestModule(t *testing.T) {
	t.Parallel()
	module := newModule("phpprobe")
	if module.Name() != "phpprobe" {
		t.Errorf("Name() = %q, want phpprobe", module.Name())
	}
	// The hooks a module leaves alone answer nil and a module writes the one
	// or two it has and inherits the rest.
	if err := module.Start(context.Background()); err != nil {
		t.Errorf("Start() = %v, want nil from the zero module", err)
	}
	if err := module.Stop(context.Background()); err != nil {
		t.Errorf("Stop() = %v, want nil from the zero module", err)
	}
}

// TestRouter covers this package's needs of a Router: a handler reaches it by
// method with a rewritten path. The registrar is the adapter. It writes to a
// recording router here.
func TestRouter(t *testing.T) {
	t.Parallel()
	recorder := &recordingRouter{}
	registrar := platformRegistrar{Router: recorder}
	registrar.Handle(http.MethodGet, "/c/{tail...}", http.NotFoundHandler())

	if recorder.method != http.MethodGet {
		t.Errorf("method = %q, want GET", recorder.method)
	}
	// The chi spelling of a catch-all, as RenderRoutePath writes it.
	if recorder.path != "/c/*" {
		t.Errorf("path = %q, want the chi spelling", recorder.path)
	}
}

// recordingRouter is a Router that records the registration a test makes. All
// other interface methods panic: any call would claim something about the
// library nothing here states.
type recordingRouter struct {
	Router
	method string
	path   string
}

// Method records the registration.
func (r *recordingRouter) Method(method, path string, handler http.Handler) {
	r.method, r.path = method, path
}
