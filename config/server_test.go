package config_test

import (
	"testing"

	"github.com/titpetric/phpscript/config"
)

// TestDefaultConfigServerBlock pins what `config/config.yml` starts the platform
// with, including the service name every trace carries.
func TestDefaultConfigServerBlock(t *testing.T) {
	t.Parallel()
	result := config.New()

	options, err := result.PlatformOptions()
	if err != nil {
		t.Fatal(err)
	}
	if options.ServerAddr != ":8080" {
		t.Errorf("addr = %q, want :8080", options.ServerAddr)
	}
	// One recorder, the platform's, on the path the telemetry block names.
	// phpscript observes the interpreter onto its traces and starts no
	// second recorder next to it.
	if !options.Telemetry.Enabled || options.Telemetry.Path != "/debug/oida" {
		t.Errorf("telemetry = %+v, want enabled on /debug/oida", options.Telemetry)
	}
	if options.Telemetry.ServiceName != "phpscript" {
		t.Errorf("service name = %q, want phpscript", options.Telemetry.ServiceName)
	}
}

// TestServerBlockConfiguresPlatformOptions covers the fields of the block a
// file can set, on top of the defaults the decoder reads it over.
func TestServerBlockConfiguresPlatformOptions(t *testing.T) {
	t.Parallel()
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
	if err := config.Unmarshal(source, &result); err != nil {
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
	// Keys the file left out keep what `config.yml` says and not the Go zero
	// value, so a partial block is usable.
	if options.Telemetry.ServiceName != "phpscript" || options.Telemetry.SampleRate != 100 {
		t.Errorf("service name = %q, sample rate = %v", options.Telemetry.ServiceName, options.Telemetry.SampleRate)
	}
}

// TestServer covers the four keys a file gets to say about the platform. The
// recorder is not among them: that one is the top level telemetry block, so a
// site cannot move the dashboard by writing a server block.
func TestServer(t *testing.T) {
	t.Parallel()
	block := config.New().Server
	if block.Addr != ":8080" {
		t.Errorf("addr = %q, want :8080", block.Addr)
	}
	if block.Quiet || len(block.Modules) != 0 || block.PidFile != "" {
		t.Errorf("block = %+v, want quiet off, every module, no pid file", block)
	}
}

// TestServer_Options covers the platform options the block describes. The
// block carries no recorder: Config.PlatformOptions adds one, so a caller
// reading the block alone starts a server that records nothing.
func TestServer_Options(t *testing.T) {
	t.Parallel()
	block := config.Server{
		Addr:    "127.0.0.1:9000",
		Quiet:   true,
		Modules: []string{"phpserver"},
		PidFile: "/run/phpscript.pid",
	}

	options := block.Options()
	if options.ServerAddr != block.Addr || !options.Quiet {
		t.Errorf("addr = %q, quiet = %t", options.ServerAddr, options.Quiet)
	}
	if len(options.Modules) != 1 || options.Modules[0] != "phpserver" {
		t.Errorf("modules = %v, want [phpserver]", options.Modules)
	}
	if options.PidFile != block.PidFile {
		t.Errorf("pid file = %q, want %q", options.PidFile, block.PidFile)
	}
	if options.Telemetry.Enabled {
		t.Error("the block configured a recorder")
	}
}

// TestNewTestConfig pins what a test inherits: a port the kernel picks and no
// recorder, so tests neither collide on a port nor record into each other.
func TestNewTestConfig(t *testing.T) {
	t.Parallel()
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
