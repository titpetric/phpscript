package config_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/config"
)

// TestTelemetry covers the inline decode of the block. The struct embeds the
// oida options with `yaml:",inline"`, so path and driver sit at one level in a
// file: a reader of `config.yml` sees one telemetry block and not a nested one.
func TestTelemetry(t *testing.T) {
	t.Parallel()
	var result config.Telemetry
	source := []byte(`
enabled: true
path: "/debug/site"
service_name: "site"
ring_buffer_size: 7
driver: disk
storage_path: "/dev/shm/site"
`)
	if err := config.Unmarshal(source, &result); err != nil {
		t.Fatal(err)
	}

	if !result.Enabled || result.Path != "/debug/site" {
		t.Errorf("options = %+v, want enabled on /debug/site", result.Options)
	}
	if result.ServiceName != "site" || result.RingBufferSize != 7 {
		t.Errorf("service name = %q, ring buffer = %d", result.ServiceName, result.RingBufferSize)
	}
	if result.Driver != "disk" || result.StoragePath != "/dev/shm/site" {
		t.Errorf("driver = %q, storage path = %q", result.Driver, result.StoragePath)
	}
}

// TestTelemetry_Validate covers the driver names a server starts under. The
// check trims and lowercases the name before it compares, so an operator who
// typed Disk is not refused over the spelling. An empty driver is memory.
func TestTelemetry_Validate(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		driver string
		fails  bool
	}{
		{driver: ""},
		{driver: "memory"},
		{driver: "disk"},
		{driver: "  Disk  "},
		{driver: "MEMORY"},
		{driver: "s3", fails: true},
		{driver: "redis", fails: true},
	} {
		t.Run(test.driver, func(t *testing.T) {
			err := config.Telemetry{Driver: test.driver}.Validate()
			if test.fails {
				if err == nil {
					t.Fatalf("driver %q validated", test.driver)
				}
				// The message carries the driver as the file spelled it, so
				// the operator can find the line.
				if !strings.Contains(err.Error(), test.driver) {
					t.Errorf("err = %q, want it to name the driver", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestTelemetry_Resolved covers the storage a driver names. Validate decides
// the driver without building anything, so this path opens a store on disk and
// nothing else does: a command that checks a file opens no directory.
func TestTelemetry_Resolved(t *testing.T) {
	t.Parallel()
	t.Run("memory holds no store", func(t *testing.T) {
		options, err := config.Telemetry{Driver: "memory"}.Resolved()
		if err != nil {
			t.Fatal(err)
		}
		if options.Storage != nil {
			t.Error("the memory driver opened a store")
		}
	})

	t.Run("an empty driver is memory", func(t *testing.T) {
		options, err := config.Telemetry{}.Resolved()
		if err != nil {
			t.Fatal(err)
		}
		if options.Storage != nil {
			t.Error("a block naming no driver opened a store")
		}
	})

	t.Run("disk opens the path it names", func(t *testing.T) {
		block := config.Telemetry{Driver: "disk", StoragePath: t.TempDir()}
		options, err := block.Resolved()
		if err != nil {
			t.Fatal(err)
		}
		if options.Storage == nil {
			t.Fatal("the disk driver opened no store")
		}
	})

	t.Run("an unknown driver carries the options back", func(t *testing.T) {
		block := config.Telemetry{Driver: "s3"}
		block.Path = "/debug/oida"
		options, err := block.Resolved()
		if err == nil {
			t.Fatal("an unknown driver resolved")
		}
		// The options come back whole beside the error, so a caller that
		// reports the failure can still name the block it came from.
		if options.Path != "/debug/oida" {
			t.Errorf("path = %q, want /debug/oida", options.Path)
		}
	})
}

// TestFlatstack covers the one key that selects the flat bytecode runtime.
// `config.yml` carries it off, because the runner is the path every
// measurement compares against; a file turns it on per application root.
func TestFlatstack(t *testing.T) {
	t.Parallel()
	if config.New().Flatstack.Enabled {
		t.Error("the compiled defaults select the flat bytecode runtime")
	}

	result := config.New()
	if err := config.Unmarshal([]byte("flatstack:\n  enabled: true\n"), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Flatstack.Enabled {
		t.Error("flatstack.enabled = false after a file set it")
	}
}

// TestRoutes covers the key that loads annotated PHP routes. `config.yml` holds
// it on, so a server started over an application root serves the @route
// annotations in it without a file saying so.
func TestRoutes(t *testing.T) {
	t.Parallel()
	if !config.New().Routes.Enabled {
		t.Error("the compiled defaults skip annotated routes")
	}

	result := config.New()
	if err := config.Unmarshal([]byte("routes:\n  enabled: false\n"), &result); err != nil {
		t.Fatal(err)
	}
	if result.Routes.Enabled {
		t.Error("routes.enabled = true after a file turned it off")
	}
}

// TestValue covers the nil pointer a caller may pass any command constructor.
// One signature covers the commands that read configuration beside the ones
// that read none, so the nil check lives here once.
func TestValue(t *testing.T) {
	t.Parallel()
	if got := config.Value(nil); got.DocumentRoot != "" || got.Routes.Enabled {
		t.Errorf("Value(nil) = %+v, want the zero configuration", got)
	}

	held := config.New()
	got := config.Value(&held)
	if got.DocumentRoot != held.DocumentRoot || got.Routes.Enabled != held.Routes.Enabled {
		t.Errorf("Value(&held) = %+v, want %+v", got, held)
	}

	// The value is a copy, so a command that edits what it holds does
	// not reach the configuration the caller still holds.
	got.DocumentRoot = "htdocs"
	if held.DocumentRoot == "htdocs" {
		t.Error("editing the returned configuration reached the pointer")
	}
}
