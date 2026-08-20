package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	config, err := loadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if config.Runner.WorkDir != "." {
		t.Fatalf("runner work directory = %q", config.Runner.WorkDir)
	}
	if config.Flatstack.Enabled {
		t.Fatal("flatstack is enabled by default")
	}
	if !config.Routes.Enabled {
		t.Fatal("annotated routes are disabled by default")
	}
	if !config.Telemetry.Enabled {
		t.Fatal("telemetry is disabled by default")
	}
	if config.Server.Addr != ":8080" {
		t.Fatalf("server options = %+v", config.Server)
	}
	if config.Telemetry.Path != "/debug/oida" ||
		config.Telemetry.ServiceName != "phpscript" ||
		config.Telemetry.RingBufferSize != 200 ||
		config.Telemetry.TopRequests != 20 ||
		config.Telemetry.SampleRate != 100 ||
		!config.Telemetry.TrackMemoryUse {
		t.Fatalf("telemetry options = %+v", config.Telemetry)
	}
}

func TestLoadConfigFile(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(filename, []byte(`
runner:
  work_dir: "app"
flatstack:
  enabled: true
routes:
  enabled: false
telemetry:
  enabled: false
env:
  - "PLATFORM_DB_APP=sqlite://app.db"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	config, err := loadConfig(filename)
	if err != nil {
		t.Fatal(err)
	}
	if config.Runner.WorkDir != "app" || !config.Flatstack.Enabled || config.Routes.Enabled || config.Telemetry.Enabled {
		t.Fatalf("config = %+v", config)
	}
	if !reflect.DeepEqual(config.Env, []string{"PLATFORM_DB_APP=sqlite://app.db"}) {
		t.Fatalf("env = %q", config.Env)
	}
}

func TestLoadConfigTelemetryDisk(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(filename, []byte(`
telemetry:
  enabled: true
  driver: disk
  storage_path: `+dir+`
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(filename)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Telemetry.Driver != "disk" || cfg.Telemetry.StoragePath != dir {
		t.Fatalf("telemetry storage = %+v", cfg.Telemetry)
	}
	opts, err := cfg.Telemetry.Resolved()
	if err != nil {
		t.Fatal(err)
	}
	if opts.Storage == nil {
		t.Fatal("disk storage not applied")
	}
}

func TestParseConfigFile(t *testing.T) {
	filename, args, err := parseConfigFile([]string{"server", "-f", "config.yml", "app"})
	if err != nil {
		t.Fatal(err)
	}
	if filename != "config.yml" || !reflect.DeepEqual(args, []string{"server", "app"}) {
		t.Fatalf("filename = %q, args = %q", filename, args)
	}
}
