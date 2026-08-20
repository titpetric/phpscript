package config_test

import (
	"testing"

	yaml "github.com/goccy/go-yaml"

	"github.com/titpetric/phpscript/config"
)

// TestDefaultConfigServerBlock pins what config/config.yml starts the platform
// with, including the service name every trace is recorded under.
func TestDefaultConfigServerBlock(t *testing.T) {
	result := config.New()

	options, err := result.PlatformOptions()
	if err != nil {
		t.Fatal(err)
	}
	if options.ServerAddr != ":8080" {
		t.Errorf("addr = %q, want :8080", options.ServerAddr)
	}
	// One recorder, the platform's, on the path the telemetry block names.
	// phpscript observes the interpreter onto its traces rather than starting a
	// second recorder next to it.
	if !options.Telemetry.Enabled || options.Telemetry.Path != "/debug/oida" {
		t.Errorf("telemetry = %+v, want enabled on /debug/oida", options.Telemetry)
	}
	if options.Telemetry.ServiceName != "phpscript" {
		t.Errorf("service name = %q, want phpscript", options.Telemetry.ServiceName)
	}
}

// TestServerBlockConfiguresPlatformOptions covers the fields of the block a
// file can set, on top of the defaults it is read over.
func TestServerBlockConfiguresPlatformOptions(t *testing.T) {
	result := config.New()
	source := []byte(`
server:
  addr: "127.0.0.1:9000"
  quiet: true
  modules: ["phpserver"]
telemetry:
  path: "/debug/service"
  ring_buffer_size: 10
`)
	if err := yaml.Unmarshal(source, &result); err != nil {
		t.Fatal(err)
	}

	options, err := result.PlatformOptions()
	if err != nil {
		t.Fatal(err)
	}
	if options.ServerAddr != "127.0.0.1:9000" || !options.Quiet {
		t.Errorf("addr = %q, quiet = %t", options.ServerAddr, options.Quiet)
	}
	if len(options.Modules) != 1 || options.Modules[0] != "phpserver" {
		t.Errorf("modules = %v", options.Modules)
	}
	if !options.Telemetry.Enabled || options.Telemetry.Path != "/debug/service" {
		t.Errorf("telemetry = %+v, want enabled on /debug/service", options.Telemetry)
	}
	if options.Telemetry.RingBufferSize != 10 {
		t.Errorf("ring_buffer_size = %d, want 10", options.Telemetry.RingBufferSize)
	}
	// Keys the file left out keep what config.yml says rather than the Go zero
	// value, which is what makes a partial block usable.
	if options.Telemetry.ServiceName != "phpscript" || options.Telemetry.SampleRate != 100 {
		t.Errorf("service name = %q, sample rate = %v", options.Telemetry.ServiceName, options.Telemetry.SampleRate)
	}
}

// TestPlatformOptionsCarryStorage pins that the driver of the telemetry block
// reaches the platform: the recorder it builds is the one that has to retain
// traces on disk.
func TestPlatformOptionsCarryStorage(t *testing.T) {
	result := config.New()
	result.Telemetry.Driver = "disk"
	result.Telemetry.StoragePath = t.TempDir()

	options, err := result.PlatformOptions()
	if err != nil {
		t.Fatal(err)
	}
	if options.Telemetry.Storage == nil {
		t.Fatal("disk storage did not reach the platform options")
	}
}

// TestPlatformOptionsRejectAnUnknownDriver pins that a bad telemetry block
// fails the server rather than starting it without the storage it asked for.
func TestPlatformOptionsRejectAnUnknownDriver(t *testing.T) {
	result := config.New()
	result.Telemetry.Driver = "s3"

	if _, err := result.PlatformOptions(); err == nil {
		t.Fatal("an unknown driver started the server")
	}
}

// TestNewTestConfig pins what a test inherits: a port the kernel picks and no
// recorder, so tests neither collide on a port nor record into each other.
func TestNewTestConfig(t *testing.T) {
	result := config.NewTestConfig()
	if result.Server.Addr != "127.0.0.1:0" || !result.Server.Quiet {
		t.Errorf("server = %+v", result.Server)
	}
	if result.Telemetry.Enabled {
		t.Error("telemetry records in tests")
	}
	if !result.Routes.Enabled {
		t.Error("annotated routes are disabled")
	}
}
