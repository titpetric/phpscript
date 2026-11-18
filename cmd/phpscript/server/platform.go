package server

import (
	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/config"
)

// This file is where this package names github.com/titpetric/platform and a
// reader of what `phpscript server` asks of the server library reads one file
// and splint's imports/pollution holds.

// LifecycleModule is platform.Module, one unit of a server's lifecycle: it
// mounts handlers, starts, and stops. The name is not Module, which this
// package already declares for the module serving one site's PHP.
type LifecycleModule = platform.Module

// ZeroModule is platform.UnimplementedModule, which answers every hook a
// module leaves alone. A module writes its one or two hooks.
type ZeroModule = platform.UnimplementedModule

// Router is platform.Router, the mux a module mounts handlers on.
type Router = platform.Router

// Service is platform.Platform, the running server a setup hook configures.
type Service = platform.Platform

// Logger is platform.Logger, where a module reports a degradation.
type Logger = platform.Logger

// TelemetryModule is platform.TelemetryModule, the recorder the platform
// builds from the options, and the recorder the interpreter observes onto.
type TelemetryModule = platform.TelemetryModule

// newZeroModule returns a zero module under name, the value Options.Modules filters on.
func newZeroModule(name string) *ZeroModule {
	return platform.NewUnimplementedModule(name)
}

// Manager is platform.Manager, which runs a server's lifecycle: it builds the
// Service a setup hook configures, serves, and reloads on a signal.
type Manager = platform.Manager

// newPlatformManager returns the manager a server runs under.
func newPlatformManager(options *config.PlatformOptions) *Manager {
	return platform.NewManager(options)
}
