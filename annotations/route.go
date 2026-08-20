package annotations

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"github.com/titpetric/platform"
)

// Route serves the @route endpoints of a PHP source tree. It is a
// platform.Module, and registers on a standard library mux just as well.
type Route struct {
	platform.UnimplementedModule
	root   fs.FS
	config config
}

// NewRoute creates a route module reading annotated endpoints from root.
func NewRoute(root fs.FS, options ...Option) *Route {
	cfg := newConfig(options...)
	return &Route{
		UnimplementedModule: *platform.NewUnimplementedModule(cfg.moduleName("phproute")),
		root:                root,
		config:              cfg,
	}
}

// Mount registers all discovered routes with the platform router.
func (r *Route) Mount(_ context.Context, router platform.Router) error {
	if router == nil {
		return fmt.Errorf("annotations: nil router")
	}
	return r.Register(platformRegistrar{Router: router})
}

// RegisterMux registers all discovered routes on a standard library mux.
func (r *Route) RegisterMux(mux *http.ServeMux) error {
	if mux == nil {
		return fmt.Errorf("annotations: nil mux")
	}
	return r.Register(serveMuxRegistrar{ServeMux: mux})
}

// Register walks the source tree and hands every @route annotation it finds to
// registrar, backed by a handler that executes the file it was declared in.
func (r *Route) Register(registrar Registrar) error {
	if registrar == nil {
		return fmt.Errorf("annotations: nil registrar")
	}
	endpoints := newHandler(r.root, r.config)
	seen := map[string]string{}
	var warnings []string
	err := scanner{root: r.root, excluded: r.config.excludedDirs}.walk(func(file string, src []byte) error {
		for _, route := range ParseRoutes(src) {
			pattern := route.Method + " " + route.Path
			if previous, ok := seen[pattern]; ok {
				warnings = append(warnings, fmt.Sprintf("duplicate route %q in %s; previously registered by %s", pattern, file, previous))
			}
			seen[pattern] = file
			registrar.Handle(route.Method, route.Path, endpoints.file(file))
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(warnings) > 0 {
		log.Println("Router loaded with warnings:")
		for _, warning := range warnings {
			log.Println("WARN", warning)
		}
	}
	return nil
}
