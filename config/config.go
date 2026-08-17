// Package config defines the config/config.yml configuration model.
package config

import (
	_ "embed"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/telemetry"
)

//go:embed config.yml
var DefaultRuntimeConfig []byte

// Config configures phpscript runtimes and HTTP modules.
type Config struct {
	Runner    runner.Options    `yaml:"runner"`
	Flatstack Flatstack         `yaml:"flatstack"`
	Routes    Routes            `yaml:"routes"`
	Telemetry telemetry.Options `yaml:"telemetry"`
	Env       []string          `yaml:"env"`
}

// Flatstack selects the flat bytecode runtime when enabled.
type Flatstack struct {
	Enabled bool `yaml:"enabled"`
}

// Routes controls loading of annotated PHP routes by HTTP servers.
type Routes struct {
	Enabled bool `yaml:"enabled"`
}

// New returns the default configuration.
func New() Config {
	options := telemetry.NewOptions()
	options.ServiceName = "phpscript"

	return Config{
		Routes: Routes{
			Enabled: true,
		},
		Telemetry: options,
	}
}
