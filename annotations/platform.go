package annotations

import (
	"github.com/titpetric/platform"
)

// This package names github.com/titpetric/platform in this file. A reader
// looking for the requirements the annotation modules place on the server
// library reads one file. splint's imports/pollution holds it here.

// Router is platform.Router, the mux a module mounts its handlers on.
// chi.Router satisfies the interface. A standard library mux behind an
// adapter satisfies the interface too. Modules take the interface.
type Router = platform.Router

// Module is platform.UnimplementedModule, the zero module every module here
// embeds. It answers the lifecycle hooks a module leaves alone. A module
// writes the few hooks it needs.
type Module = platform.UnimplementedModule

// newModule returns the embedded module registered under name.
func newModule(name string) *Module {
	return platform.NewUnimplementedModule(name)
}
