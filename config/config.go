// Package config defines the config/config.yml configuration model.
package config

import (
	"fmt"
	"strings"

	_ "embed"

	yaml "github.com/goccy/go-yaml"
	"github.com/titpetric/platform"

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
	Server    Server         `yaml:"server"`
	Telemetry Telemetry      `yaml:"telemetry"`
	Env       []string       `yaml:"env"`

	// DocumentRoot is the directory beneath the application root that is
	// served over HTTP. It is "public" and almost never worth setting; it
	// exists for a site whose tree already names that directory something
	// else.
	DocumentRoot string `yaml:"document_root"`

	// VirtualHost is the list of sites this server routes by domain. Each
	// entry names an application root whose phpscript.yml is the
	// configuration that site runs under.
	VirtualHost []VirtualHost `yaml:"virtualhost"`
}

// PlatformOptions returns the platform options `phpscript server` runs under.
//
// There is one recorder and the platform owns it: given these options it
// builds the tracer, wraps every module in the tracing middleware and mounts
// the debug front end under Telemetry.Path. phpscript registers no recorder of
// its own. It observes the interpreter onto the trace that middleware started,
// so interpreter spans show up on the same front end as the request that
// caused them.
func (c Config) PlatformOptions() (*platform.Options, error) {
	options, err := c.Telemetry.Resolved()
	if err != nil {
		return nil, err
	}
	result := c.Server.Options()
	result.Telemetry = options
	return result, nil
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

// New returns the default configuration, which is config/config.yml as it is
// compiled into the binary. Defaults live in that file and nowhere else, so a
// file passed with -f is read on top of it and only has to name what it
// changes.
func New() Config {
	var result Config
	if err := yaml.Unmarshal(DefaultRuntimeConfig, &result); err != nil {
		panic(fmt.Errorf("config: embedded config.yml: %w", err))
	}
	return result
}

// NewTestConfig returns the default configuration with the parts a test has no
// use for turned off: a port picked by the kernel, no lifecycle logging, and no
// recorder, so parallel tests neither collide on a port nor record into each
// other.
func NewTestConfig() Config {
	result := New()
	result.Server.Addr = "127.0.0.1:0"
	result.Server.Quiet = true
	result.Telemetry.Enabled = false
	return result
}
