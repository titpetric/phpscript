package config

import (
	"github.com/titpetric/platform"
)

// Server is the YAML spelling of platform.Options: everything a configuration
// file gets to say about the platform `phpscript server` runs on. The options
// the platform takes from the process rather than from a file, such as the
// config filesystem a composed service embeds, are not part of it, and neither
// is the recorder: that one is the top level telemetry block, applied by
// Config.PlatformOptions.
type Server struct {
	// Addr is the address the HTTP server listens on.
	Addr string `yaml:"addr"`

	// Quiet turns down platform lifecycle logging.
	Quiet bool `yaml:"quiet"`

	// Modules limits which platform modules load, by name. An empty list
	// loads all of them.
	Modules []string `yaml:"modules"`
}

// Options returns the platform options this block describes, with no recorder
// configured. Callers want Config.PlatformOptions, which adds one.
func (s Server) Options() *platform.Options {
	return &platform.Options{
		ServerAddr: s.Addr,
		Quiet:      s.Quiet,
		Modules:    s.Modules,
	}
}
