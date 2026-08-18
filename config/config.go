// Package config defines the config/config.yml configuration model.
package config

import (
	"fmt"
	"strings"

	_ "embed"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/telemetry"
)

//go:embed config.yml
var DefaultRuntimeConfig []byte

// Config configures phpscript runtimes and HTTP modules.
type Config struct {
	Runner    runner.Options `yaml:"runner"`
	Flatstack Flatstack      `yaml:"flatstack"`
	Routes    Routes         `yaml:"routes"`
	Telemetry Telemetry      `yaml:"telemetry"`
	Env       []string       `yaml:"env"`
}

// Telemetry is oida options plus optional durable storage.
type Telemetry struct {
	telemetry.Options `yaml:",inline"`
	Driver            string `yaml:"driver"`
	StoragePath       string `yaml:"storage_path"`
}

// Resolved returns oida options with storage applied.
func (t Telemetry) Resolved() (telemetry.Options, error) {
	opts := t.Options
	switch strings.ToLower(strings.TrimSpace(t.Driver)) {
	case "", "memory":
		return opts, nil
	case "disk":
		path := t.StoragePath
		if path == "" {
			path = "/dev/shm/phpscript-trace-detail"
		}
		limit := opts.RingBufferSize
		if limit <= 0 {
			limit = 200
		}
		store, err := telemetry.NewStorageDisk(limit, path)
		if err != nil {
			return opts, fmt.Errorf("telemetry disk storage: %w", err)
		}
		opts.Storage = store
		return opts, nil
	default:
		return opts, fmt.Errorf("telemetry driver %q: want memory or disk", t.Driver)
	}
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
		Telemetry: Telemetry{Options: options},
	}
}
