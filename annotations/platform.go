package annotations

import (
	"github.com/titpetric/platform"
)

// This file is where this package names github.com/titpetric/platform, so a
// reader of what the annotation modules ask of the server library reads one
// file and splint's imports/pollution holds.

// Router is platform.Router, the mux a module mounts its handlers on. A
// chi.Router satisfies it, and so does a standard library mux behind an
// adapter, so a module takes the interface.
type Router = platform.Router

// Module is platform.UnimplementedModule, the zero module every module here
// embeds: it answers the lifecycle hooks a module leaves alone, so a module
// writes the one or two it has.
type Module = platform.UnimplementedModule

// newModule returns the embedded module under name, the name the platform
// reports a module by and Options.Modules filters on.
func newModule(name string) *Module {
	return platform.NewUnimplementedModule(name)
}
