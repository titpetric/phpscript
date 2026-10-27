package annotations

import (
	"net/http"

	"github.com/titpetric/phpscript/model"
)

// Registrar binds a handler to an HTTP method and path, the one type a
// discovered route and whichever router the application runs both speak.
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
	// ServeMux panics on a pattern it cannot parse and an unrendered
	// {id:[0-9]+} takes the process down at registration.
	r.ServeMux.Handle(method+" "+model.RenderRoutePath(path, model.RouteServeMux), handler)
}

// platformRegistrar registers routes on a platform router.
type platformRegistrar struct {
	Router
}

func (r platformRegistrar) Handle(method string, path string, handler http.Handler) {
	r.Router.Method(method, model.RenderRoutePath(path, model.RouteChi), handler)
}
