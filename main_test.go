package main

import "testing"

func TestLoadConfig(t *testing.T) {
	config, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Runner.WorkDir != "." {
		t.Fatalf("runner work directory = %q", config.Runner.WorkDir)
	}
	if config.Flatstack.Enabled {
		t.Fatal("flatstack is enabled by default")
	}
	if !config.Status.Enabled {
		t.Fatal("status module is disabled by default")
	}
	if config.Status.Options.RingBufferSize != 100 ||
		config.Status.Options.TopRequests != 20 ||
		!config.Status.Options.TrackMemoryUse {
		t.Fatalf("status options = %+v", config.Status.Options)
	}
}
