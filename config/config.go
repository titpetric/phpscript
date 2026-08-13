// Package config defines the config/config.yml configuration model.
package config

import (
	_ "embed"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/status"
)

//go:embed config.yml
var DefaultRuntimeConfig []byte

// Config configures phpscript runtimes and HTTP modules.
type Config struct {
	Runner    runner.Options `yaml:"runner"`
	Flatstack Flatstack      `yaml:"flatstack"`
	Routes    Routes         `yaml:"routes"`
	Status    Status         `yaml:"status"`
	Env       []string       `yaml:"env"`
}

// Flatstack selects the flat bytecode runtime when enabled.
type Flatstack struct {
	Enabled bool `yaml:"enabled"`
}

// Routes controls loading of annotated PHP routes by HTTP servers.
type Routes struct {
	Enabled bool `yaml:"enabled"`
}

// Status configures the optional server status module.
type Status struct {
	Enabled bool           `yaml:"enabled"`
	Options status.Options `yaml:"options"`
}

// New returns the default configuration.
func New() Config {
	return Config{
		Routes: Routes{
			Enabled: true,
		},
		Status: Status{
			Enabled: true,
			Options: status.NewOptions(),
		},
	}
}
