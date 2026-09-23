package server

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/titpetric/platform"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/internal/flags"
)

// site writes an application root serving body at its index, and returns the
// root.
func site(t *testing.T, name, body string) string {
	t.Helper()

	root := filepath.Join(t.TempDir(), name)
	public := filepath.Join(root, "public")
	if err := os.MkdirAll(public, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(public, "index.php"), []byte("<?php echo \""+body+"\";"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "phpscript.yml"), []byte("telemetry:\n  enabled: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// operatorConfig writes the file the server is started with, and returns its
// path. Rewriting it and reloading is what these tests are about.
func operatorConfig(t *testing.T, source string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// body fetches the index of one virtual host.
func body(t *testing.T, url, host string) (int, string) {
	t.Helper()

	request, err := http.NewRequest(http.MethodGet, url+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = host

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	buf := make([]byte, 64)
	n, _ := response.Body.Read(buf)
	return response.StatusCode, string(buf[:n])
}

// TestReload covers what SIGHUP does, driving Reload directly so nothing
// depends on signal delivery timing.
func TestReload(t *testing.T) {
	a := site(t, "a", "a")
	b := site(t, "b", "b")

	header := "telemetry:\n  enabled: false\nserver:\n  addr: \"127.0.0.1:0\"\n"
	oneSite := header + "virtualhost:\n  - domain: a.localhost\n    root: " + a + "\n"
	twoSites := oneSite + "  - domain: b.localhost\n    root: " + b + "\n"

	start := func(t *testing.T, filename string) *managerFixture {
		t.Helper()

		globals := &flags.Options{ConfigFile: filename}
		appConfig, err := config.Load(filename)
		if err != nil {
			t.Fatal(err)
		}

		manager, err := newManager(t.Context(), nil, appConfig, globals)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(manager.Stop)
		if err := manager.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		return &managerFixture{manager: manager, filename: filename}
	}

	t.Run("a reload reads the file again", func(t *testing.T) {
		filename := operatorConfig(t, oneSite)
		fixture := start(t, filename)

		if status, got := body(t, fixture.manager.URL(), "a.localhost"); status != 200 || got != "a" {
			t.Fatalf("a.localhost = %d %q", status, got)
		}
		if status, _ := body(t, fixture.manager.URL(), "b.localhost"); status != 404 {
			t.Fatalf("b.localhost = %d, want 404 before the reload", status)
		}

		fixture.rewrite(t, twoSites)
		if err := fixture.manager.Reload(t.Context()); err != nil {
			t.Fatal(err)
		}

		// The site the edit added, and the one that was already there.
		if status, got := body(t, fixture.manager.URL(), "b.localhost"); status != 200 || got != "b" {
			t.Fatalf("b.localhost = %d %q, want the reloaded site", status, got)
		}
		if status, got := body(t, fixture.manager.URL(), "a.localhost"); status != 200 || got != "a" {
			t.Fatalf("a.localhost = %d %q", status, got)
		}
	})

	t.Run("the address survives a reload", func(t *testing.T) {
		filename := operatorConfig(t, oneSite)
		fixture := start(t, filename)

		before := fixture.manager.URL()
		fixture.rewrite(t, twoSites)
		if err := fixture.manager.Reload(t.Context()); err != nil {
			t.Fatal(err)
		}
		if after := fixture.manager.URL(); after != before {
			t.Fatalf("url = %q, want %q", after, before)
		}
	})

	// The reason Check exists. A reload that cannot be applied leaves the
	// generation that is serving alone, rather than stopping it first and
	// then discovering the new configuration is unusable.
	t.Run("a broken configuration is refused and keeps serving", func(t *testing.T) {
		filename := operatorConfig(t, oneSite)
		fixture := start(t, filename)

		serving := fixture.manager.Platform()

		fixture.rewrite(t, twoSites+"  - domain: c.localhost\n    root: /nonexistent-root\n")
		err := fixture.manager.Reload(t.Context())
		if err == nil {
			t.Fatal("a reload with a missing root was applied")
		}

		if fixture.manager.Platform() != serving {
			t.Fatal("the generation that was serving was replaced by a refused reload")
		}
		if status, got := body(t, fixture.manager.URL(), "a.localhost"); status != 200 || got != "a" {
			t.Fatalf("a.localhost = %d %q, want the old generation still serving", status, got)
		}
	})
}

// managerFixture is a started manager and the file it reads.
type managerFixture struct {
	manager  *platform.Manager
	filename string
}

func (f *managerFixture) rewrite(t *testing.T, source string) {
	t.Helper()

	if err := os.WriteFile(f.filename, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}
