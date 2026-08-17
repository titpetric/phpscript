package annotations

import (
	"net/http"

	"github.com/titpetric/platform"
)

// Registrar binds a handler to an HTTP method and path. It is the seam between
// discovered routes and whichever router the application runs.
type Registrar interface {
	Handle(method string, path string, handler http.Handler)
}

// serveMuxRegistrar registers routes on a standard library mux.
type serveMuxRegistrar struct {
	*http.ServeMux
}

func (r serveMuxRegistrar) Handle(method string, path string, handler http.Handler) {
	if path == "/" {
		// Without the {$} terminator the root pattern matches every unrouted
		// path, turning a 404 into an index page.
		path = "/{$}"
	}
	r.ServeMux.Handle(method+" "+path, handler)
}

// platformRegistrar registers routes on a platform router.
type platformRegistrar struct {
	platform.Router
}

func (r platformRegistrar) Handle(method string, path string, handler http.Handler) {
	r.Router.Method(method, path, handler)
}
