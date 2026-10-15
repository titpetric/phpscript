package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/stdlib/mail"
)

// TestOverlay covers the layering every phpscript.yml gets: a file names what
// it changes and inherits the rest. A missing file is an error with no fall
// back to base, because a tree served under settings nobody wrote is the
// failure mode worth avoiding.
func TestOverlay(t *testing.T) {
	t.Parallel()
	base := config.New()

	t.Run("a file names what it changes", func(t *testing.T) {
		dir := t.TempDir()
		filename := filepath.Join(dir, "phpscript.yml")
		if err := os.WriteFile(filename, []byte("routes:\n  enabled: false\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		result, declared, err := config.Overlay(base, filename, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		if result.Routes.Enabled {
			t.Error("routes.enabled = true, the file turned it off")
		}
		// A key the file left out keeps the operator's value.
		if result.Telemetry.ServiceName != base.Telemetry.ServiceName {
			t.Errorf("service name = %q, want %q", result.Telemetry.ServiceName, base.Telemetry.ServiceName)
		}
		if _, ok := declared["routes"]; !ok {
			t.Errorf("declared = %+v, want it to name routes", declared)
		}
		if _, ok := declared["telemetry"]; ok {
			t.Errorf("declared = %+v, want nothing for a key the file left out", declared)
		}
	})

	t.Run("a missing file fails and base comes back", func(t *testing.T) {
		filename := filepath.Join(t.TempDir(), "absent.yml")
		result, declared, err := config.Overlay(base, filename, nil, "")
		if err == nil {
			t.Fatal("a missing file loaded")
		}
		if declared != nil {
			t.Errorf("declared = %+v, want nothing", declared)
		}
		if result.Telemetry.ServiceName != base.Telemetry.ServiceName {
			t.Errorf("service name = %q, want base to come back whole", result.Telemetry.ServiceName)
		}
	})
}

// TestOverlayBytes covers Overlay over source a caller already read. A fixture
// tree is compiled into the test binary, so the bytes arrive from an fs.FS with
// no path on disk; filename is only what errors name.
func TestOverlayBytes(t *testing.T) {
	t.Parallel()
	base := config.New()

	t.Run("a forbidden key is refused and named", func(t *testing.T) {
		source := []byte("server:\n  addr: \"127.0.0.1:9999\"\n")
		_, declared, err := config.OverlayBytes(base, "site.yml", source, []string{"server"}, "is set by the operator")
		if err == nil {
			t.Fatal("a forbidden key loaded")
		}
		want := `site.yml: "server" is set by the operator`
		if err.Error() != want {
			t.Errorf("err = %q, want %q", err.Error(), want)
		}
		// The keys the file named come back beside the refusal, so a caller
		// reporting it can say what else the file set.
		if _, ok := declared["server"]; !ok {
			t.Errorf("declared = %+v, want it to name server", declared)
		}
	})

	t.Run("a mapping replaces the inherited one", func(t *testing.T) {
		operator := config.New()
		operator.Mail = config.Mail{"default": mail.Config{
			Host:     "mail.operator.example",
			From:     "operator@example.com",
			Password: "operator-secret",
		}}

		source := []byte("mail:\n  marketing:\n    host: mail.site.example\n    from: site@example.com\n")
		result, _, err := config.OverlayBytes(operator, "site.yml", source, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Mail) != 1 {
			t.Fatalf("mail = %+v, want the one server the file named", result.Mail)
		}
		if _, ok := result.Mail["default"]; ok {
			t.Error("the operator's server survived a file that named its own")
		}
	})

	t.Run("naming mail with nothing under it is no servers", func(t *testing.T) {
		operator := config.New()
		operator.Mail = config.Mail{"default": mail.Config{Host: "mail.operator.example", From: "operator@example.com"}}

		// A null node is the one spelling the decoder leaves as the inherited
		// value, so the guard in OverlayBytes is what empties it.
		result, _, err := config.OverlayBytes(operator, "site.yml", []byte("mail:\n"), nil, "")
		if err != nil {
			t.Fatal(err)
		}
		if result.Mail != nil {
			t.Errorf("mail = %+v, want none", result.Mail)
		}
	})

	t.Run("a flat mail block is refused", func(t *testing.T) {
		source := []byte("mail:\n  host: mail.site.example\n")
		if _, _, err := config.OverlayBytes(base, "site.yml", source, nil, ""); err == nil {
			t.Fatal("a flat mail block loaded")
		}
	})

	t.Run("source that is not yaml names the file", func(t *testing.T) {
		_, _, err := config.OverlayBytes(base, "site.yml", []byte("routes: [:\n"), nil, "")
		if err == nil {
			t.Fatal("a broken document loaded")
		}
		if !strings.Contains(err.Error(), "site.yml") {
			t.Errorf("err = %q, want it to name the file", err.Error())
		}
	})
}

// TestDeclares covers telling a setting a file named from one it inherited
// through Overlay. Validation reads it for keys whose inherited value is legal
// and whose declared value is not: a site may carry the operator's telemetry
// path, and may not name it.
func TestDeclares(t *testing.T) {
	t.Parallel()
	declared := map[string]any{
		"autoindex": true,
		"telemetry": map[string]any{"path": "/debug/site"},
		"routes":    "enabled",
	}

	for _, test := range []struct {
		name string
		keys []string
		want bool
	}{
		{name: "a top level key", keys: []string{"autoindex"}, want: true},
		{name: "a nested key", keys: []string{"telemetry", "path"}, want: true},
		{name: "a key the file left out", keys: []string{"server"}, want: false},
		{name: "a nested key the block left out", keys: []string{"telemetry", "driver"}, want: false},
		{name: "a path through a scalar", keys: []string{"routes", "enabled"}, want: false},
		{name: "no keys at all", keys: nil, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := config.Declares(declared, test.keys...); got != test.want {
				t.Errorf("Declares(%v) = %t, want %t", test.keys, got, test.want)
			}
		})
	}
}
