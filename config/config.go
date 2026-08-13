// Package config defines the phpscript.yml configuration model.
package config

import (
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib/status"
)

// Config configures phpscript runtimes and HTTP modules.
type Config struct {
	Runner    runner.Options `yaml:"runner"`
	Flatstack Flatstack      `yaml:"flatstack"`
	Status    Status         `yaml:"status"`
}

// Flatstack selects the flat bytecode runtime when enabled.
type Flatstack struct {
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
		Status: Status{
			Enabled: true,
			Options: status.NewOptions(),
		},
	}
}
