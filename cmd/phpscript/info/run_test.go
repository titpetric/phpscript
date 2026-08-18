package info

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
