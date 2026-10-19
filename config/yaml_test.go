package config_test

import (
	"testing"

	"github.com/titpetric/phpscript/config"
)

// TestUnmarshal covers the one decoder every configuration reaches: a
// document names the keys it sets, the value it holds arrives typed, and the
// decoder passes over a key the struct does not declare.
func TestUnmarshal(t *testing.T) {
	t.Parallel()
	var result struct {
		Server struct {
			Addr string `yaml:"addr"`
			Port int    `yaml:"port"`
		} `yaml:"server"`
		Enabled bool `yaml:"enabled"`
	}

	document := []byte("server:\n  addr: 127.0.0.1\n  port: 8080\nenabled: true\nunknown: 3\n")
	if err := config.Unmarshal(document, &result); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got, want := result.Server.Addr, "127.0.0.1"; got != want {
		t.Errorf("server.addr = %q, want %q", got, want)
	}
	if got, want := result.Server.Port, 8080; got != want {
		t.Errorf("server.port = %d, want %d", got, want)
	}
	if !result.Enabled {
		t.Error("enabled = false, want true")
	}
}

// TestUnmarshalError covers what a malformed document answers: an error, and
// not a zero value a caller would serve under.
func TestUnmarshalError(t *testing.T) {
	t.Parallel()
	var result map[string]any

	if err := config.Unmarshal([]byte("server:\n\taddr: x\n"), &result); err == nil {
		t.Error("Unmarshal = nil on a document indented with a tab, want an error")
	}
}

// TestUnmarshalDefaultConfig covers the embedded defaults going through the
// same door: `config.yml` is a document this decoder reads, and New() panics if
// it stops being one.
func TestUnmarshalDefaultConfig(t *testing.T) {
	t.Parallel()
	var result config.Config

	if err := config.Unmarshal(config.DefaultRuntimeConfig, &result); err != nil {
		t.Fatalf("Unmarshal(DefaultRuntimeConfig): %v", err)
	}
	if result.Server.Addr == "" {
		t.Error("the embedded defaults named no server address")
	}
}
