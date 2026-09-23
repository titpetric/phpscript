package server_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/cmd/phpscript/server"
)

// writeConfig puts an operator configuration in a temporary directory and
// returns its path.
func writeConfig(t *testing.T, source string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// checkResult runs Check and returns what it wrote and whether it passed.
func checkResult(t *testing.T, filename, root string) (out, errOut string, ok bool) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	err := server.Check(filename, root, &stdout, &stderr)
	return stdout.String(), stderr.String(), err == nil
}

func TestCheck(t *testing.T) {
	t.Run("a good single root passes", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "public"), 0o755); err != nil {
			t.Fatal(err)
		}

		out, errOut, ok := checkResult(t, "", root)
		if !ok {
			t.Fatalf("check failed: %s", errOut)
		}
		if want := "built-in defaults: ok\n"; out != want {
			t.Errorf("stdout = %q, want %q", out, want)
		}
	})

	// The gap this closes: fs.Sub does not stat, so a single-root server
	// used to come up and answer 404 on the first request instead.
	t.Run("a missing document root fails", func(t *testing.T) {
		_, errOut, ok := checkResult(t, "", t.TempDir())
		if ok {
			t.Fatal("a root with no public/ passed")
		}
		if !strings.Contains(errOut, "document root") {
			t.Errorf("stderr = %q, want it to name the document root", errOut)
		}
		if !strings.Contains(errOut, "built-in defaults: failed") {
			t.Errorf("stderr = %q, want a verdict", errOut)
		}
	})

	t.Run("a configuration that does not parse fails", func(t *testing.T) {
		filename := writeConfig(t, "server:\n  addr: [not, a, string]\n")

		_, errOut, ok := checkResult(t, filename, "")
		if ok {
			t.Fatal("a broken configuration passed")
		}
		if !strings.Contains(errOut, filepath.Base(filename)+": failed") {
			t.Errorf("stderr = %q, want a verdict naming the file", errOut)
		}
	})

	t.Run("a flat mail block is reported in its own words", func(t *testing.T) {
		filename := writeConfig(t, "mail:\n  host: mail.example.com\n")

		_, errOut, ok := checkResult(t, filename, "")
		if ok {
			t.Fatal("a flat mail block passed")
		}
		if !strings.Contains(errOut, "is not a server name") {
			t.Errorf("stderr = %q, want the flat block refusal", errOut)
		}
	})

	t.Run("a broken virtual host fails", func(t *testing.T) {
		filename := writeConfig(t, "telemetry:\n  enabled: false\nvirtualhost:\n  - domain: a.test\n    root: /nonexistent-root\n")

		_, errOut, ok := checkResult(t, filename, "")
		if ok {
			t.Fatal("a virtual host with no root passed")
		}
		if !strings.Contains(errOut, "a.test") {
			t.Errorf("stderr = %q, want it to name the site", errOut)
		}
	})

	t.Run("a root beside virtual hosts is refused", func(t *testing.T) {
		filename := writeConfig(t, "telemetry:\n  enabled: false\nvirtualhost:\n  - domain: a.test\n    root: /srv/a\n")

		_, errOut, ok := checkResult(t, filename, "./site")
		if ok {
			t.Fatal("a root beside virtual hosts passed")
		}
		if !strings.Contains(errOut, "has no virtual host to belong to") {
			t.Errorf("stderr = %q, want the refusal", errOut)
		}
	})

	t.Run("an unknown telemetry driver fails", func(t *testing.T) {
		filename := writeConfig(t, "telemetry:\n  driver: elsewhere\n")

		_, errOut, ok := checkResult(t, filename, "")
		if ok {
			t.Fatal("an unknown driver passed")
		}
		if !strings.Contains(errOut, "want memory or disk") {
			t.Errorf("stderr = %q", errOut)
		}
	})

	// A test block the server never reads still fails -t, because -t is a
	// question about the file rather than about one command.
	t.Run("a test block the run could not use fails", func(t *testing.T) {
		filename := writeConfig(t, "test:\n  cache: sometimes\n")

		_, errOut, ok := checkResult(t, filename, "")
		if ok {
			t.Fatal("an unusable test.cache passed")
		}
		if !strings.Contains(errOut, "test.cache") {
			t.Errorf("stderr = %q", errOut)
		}
	})

	// Resolved builds the trace store on disk; Validate must not. A test of
	// a configuration that created a directory, owned by whoever ran the
	// test rather than by the service, is a side effect nobody asked for.
	t.Run("disk telemetry creates no storage", func(t *testing.T) {
		storage := filepath.Join(t.TempDir(), "traces")
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "public"), 0o755); err != nil {
			t.Fatal(err)
		}

		filename := writeConfig(t, "telemetry:\n  driver: disk\n  storage_path: "+storage+"\n")

		if _, errOut, ok := checkResult(t, filename, root); !ok {
			t.Fatalf("check failed: %s", errOut)
		}
		if _, err := os.Stat(storage); err == nil {
			t.Fatalf("%s was created by a configuration test", storage)
		}
	})
}
