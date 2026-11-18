// Package `config` defines the configuration model in `config/config.yml`.
package config

import (
	"fmt"
	"strings"

	_ "embed"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/telemetry"
)

// DefaultRuntimeConfig is the `config/config.yml` compiled into the binary.
// That file holds every default alone. The decoder reads a file passed with -f
// on top. That file only names what changes.
//
//go:embed "config.yml"
var DefaultRuntimeConfig []byte

// Config configures phpscript runtimes and HTTP modules.
type Config struct {
	Runner    runner.Options `yaml:"runner"`
	Flatstack Flatstack      `yaml:"flatstack"`
	Routes    Routes         `yaml:"routes"`
	Server    Server         `yaml:"server"`
	Telemetry Telemetry      `yaml:"telemetry"`
	Test      Test           `yaml:"test"`
	Env       []string       `yaml:"env"`

	// Mail configures the mail servers mail() and `new Mail($name)` send
	// through, keyed by the name a script names.
	Mail Mail `yaml:"mail"`

	// DocumentRoot is the directory beneath the application root the server
	// publishes over HTTP. The default is "public" and almost never worth
	// setting; the key exists for a site whose tree already names that
	// directory something else.
	DocumentRoot string `yaml:"document_root"`

	// Autoindex answers a directory lacking an index page by listing the
	// contents, as nginx's autoindex does. The default is off. Publishing every
	// file below the document root is a decision a site makes; a site should
	// not arrive at it by leaving an index.html out.
	Autoindex bool `yaml:"autoindex"`

	// VirtualHost is the list of sites this server routes by domain. Each
	// entry names an application root whose phpscript.yml is the
	// configuration that site runs under.
	VirtualHost []VirtualHost `yaml:"virtualhost"`
}

// PlatformOptions returns the platform options `phpscript server` runs under.
func (c Config) PlatformOptions() (*PlatformOptions, error) {
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

// Check reports a telemetry block the server could not run under, without
// building the storage the driver names, which Resolved does.
func (t Telemetry) Check() error {
	switch strings.ToLower(strings.TrimSpace(t.Driver)) {
	case "", "memory", "disk":
		return nil
	default:
		return fmt.Errorf("telemetry driver %q: want memory or disk", t.Driver)
	}
}

// Resolved returns oida options with storage applied.
func (t Telemetry) Resolved() (telemetry.Options, error) {
	if err := t.Check(); err != nil {
		return t.Options, err
	}

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

// New returns the default configuration, the `config/config.yml` the build
// compiled into the binary.
func New() Config {
	var result Config
	if err := Unmarshal(DefaultRuntimeConfig, &result); err != nil {
		panic(fmt.Errorf("config: embedded config.yml: %w", err))
	}
	return result
}

// NewTestConfig returns New with a kernel-picked port, no logging, no recorder.
func NewTestConfig() Config {
	result := New()
	result.Server.Addr = "127.0.0.1:0"
	result.Server.Quiet = true
	result.Telemetry.Enabled = false
	return result
}

// Value returns the configuration a pointer holds, and the zero configuration
// for a nil pointer. No command repeats the nil check.
func Value(c *Config) Config {
	if c == nil {
		return Config{}
	}
	return *c
}
