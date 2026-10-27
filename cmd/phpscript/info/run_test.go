package info

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/internal/flags"
)

func TestRunPrintsRuntime(t *testing.T) {
	out := captureStdout(t, func() {
		if err := Run(context.Background(), nil, Options{}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "# phpscript") || !strings.Contains(out, "Runtime => phpscript") {
		t.Fatalf("info output:\n%s", out)
	}
	if strings.Contains(out, "# Classes") {
		t.Fatalf("verbose section without -v:\n%s", out)
	}
}

func TestRunVerboseListsBindings(t *testing.T) {
	out := captureStdout(t, func() {
		if err := Run(context.Background(), nil, Options{Verbose: true}); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"# Classes", "## Database", "new Database", "## Functions", "`phpinfo()`"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestRunSourceTree(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "sample.php")
	if err := os.WriteFile(src, []byte("<?php\nclass Sample {\n\tfunction run($x) {}\n}\nfunction helper() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := Run(context.Background(), []string{dir}, Options{}); err != nil {
			t.Fatal(err)
		}
	})
	for _, want := range []string{"# Source", "### Sample", "function run($x)", "function helper()"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

// TestNewCommand covers what the constructor carries into the command: the
// shared -v is this command's verbosity dial and the Run closure has to read
// it off the global options main filled, and not off a flag of its own. A
// command built with -v set prints the binding sections, and one built without
// it prints the summary alone.
func TestNewCommand(t *testing.T) {
	command := NewCommand(nil, &flags.Options{})

	if command.Name != "info" {
		t.Errorf("Name = %q, want %q", command.Name, "info")
	}
	if command.Title != Name {
		t.Errorf("Title = %q, want %q", command.Title, Name)
	}
	if command.Bind != nil {
		t.Error("Bind is set, want nil: -v is the shared flag, not one of this command's")
	}

	quiet := captureStdout(t, func() {
		if err := command.Run(context.Background(), nil); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if strings.Contains(quiet, "# Classes") {
		t.Errorf("a command built without -v printed the binding sections:\n%s", quiet)
	}

	verbose := captureStdout(t, func() {
		if err := NewCommand(nil, &flags.Options{Verbose: true}).Run(context.Background(), nil); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if !strings.Contains(verbose, "# Classes") {
		t.Errorf("a command built with -v did not print the binding sections:\n%s", verbose)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
