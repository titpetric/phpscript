package annotations_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/annotations"
)

// recordingService is a Router that records every registration. Every other
// method of the interface panics: a call to one would be a claim on the server
// library that nothing here states.
type recordingService struct {
	annotations.Router
	registered []string
}

// Method records one registration.
func (r *recordingService) Method(method string, path string, _ http.Handler) {
	r.registered = append(r.registered, method+" "+path)
}

// TestBindings covers the deferral both ways around. A registration before the
// mount reaches the router in order once it arrives. A registration after the
// mount goes straight there.
func TestBindings(t *testing.T) {
	t.Parallel()
	bindings := annotations.NewBindings()
	bindings.Handle(http.MethodGet, "/first", http.NotFoundHandler())
	bindings.Handle(http.MethodPost, "/second", http.NotFoundHandler())

	service := &recordingService{}
	if err := bindings.Mount(context.Background(), service); err != nil {
		t.Fatal(err)
	}
	bindings.Handle(http.MethodGet, "/third", http.NotFoundHandler())

	want := "GET /first,POST /second,GET /third"
	if got := strings.Join(service.registered, ","); got != want {
		t.Errorf("registered %q, want %q", got, want)
	}
}

// TestBindings_Name covers the name the platform reports the module by, which
// server.modules filters on and a virtual host suffixes per site.
func TestBindings_Name(t *testing.T) {
	t.Parallel()
	if got := annotations.NewBindings().Name(); got != "phpbindings" {
		t.Errorf("Name() = %q, want phpbindings", got)
	}
	suffixed := annotations.NewBindings(annotations.WithModuleSuffix("shop.example.com"))
	if got := suffixed.Name(); got != "phpbindings:shop.example.com" {
		t.Errorf("Name() = %q, want the per-site name", got)
	}
}

// TestBindings_RejectsNilRouter covers the mount a host gets wrong: the routes
// a job bound would go nowhere and the error says so.
func TestBindings_RejectsNilRouter(t *testing.T) {
	t.Parallel()
	if err := annotations.NewBindings().Mount(context.Background(), nil); err == nil {
		t.Error("a nil router was accepted, so the bound routes went nowhere")
	}
}
