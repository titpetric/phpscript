package config_test

import (
	"testing"

	"github.com/titpetric/phpscript/config"
)

// TestPlatformOptions covers what the alias stands for: the options a server
// starts under, assembled from the server block and the telemetry one. The
// type is platform.Options, which this package names in one file alone.
func TestPlatformOptions(t *testing.T) {
	t.Parallel()
	result := config.New()
	result.Server.Addr = "127.0.0.1:8080"

	var options *config.PlatformOptions
	options, err := result.PlatformOptions()
	if err != nil {
		t.Fatal(err)
	}
	if options.ServerAddr != "127.0.0.1:8080" {
		t.Errorf("ServerAddr = %q, want the address the block names", options.ServerAddr)
	}
}

// TestPlatformOptionsCarryStorage pins that the driver of the telemetry block
// reaches the platform: the recorder it builds is the one that has to keep
// traces on disk.
func TestPlatformOptionsCarryStorage(t *testing.T) {
	t.Parallel()
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
// fails the server and never starts it without the storage it named.
func TestPlatformOptionsRejectAnUnknownDriver(t *testing.T) {
	t.Parallel()
	result := config.New()
	result.Telemetry.Driver = "s3"

	if _, err := result.PlatformOptions(); err == nil {
		t.Fatal("an unknown driver started the server")
	}
}
