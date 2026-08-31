package list

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunListsPaths covers the default behaviour: a markdown table of the PHP
// files a path selects.
func TestRunListsPaths(t *testing.T) {
	dir := t.TempDir()
	source := "<?php\n// @route GET /hello\nclass Greeter {}\n"
	if err := os.WriteFile(filepath.Join(dir, "hello.php"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := run(&out, []string{dir}, false); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"Route", "Filename", "Classes", "GET /hello", "hello.php", "Greeter"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output does not contain %q:\n%s", want, out.String())
		}
	}
}

// TestRunListsStdlib covers --stdlib: the registered surface of this build,
// with no tree involved.
func TestRunListsStdlib(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, nil, true); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"| Kind", "| function |", "| class    |", "| constant |", "fnmatch", "glob"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output does not contain %q", want)
		}
	}
}

// TestRunRejectsStdlibWithPaths pins the refusal. The two modes answer about
// different things, so a caller that named both meant one of them and should be
// told which one was dropped rather than guessed at.
func TestRunRejectsStdlibWithPaths(t *testing.T) {
	var out bytes.Buffer
	err := run(&out, []string{"."}, true)
	if err == nil {
		t.Fatal("run returned nil error for --stdlib with a path argument")
	}
	if want := "list --stdlib takes no path arguments"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err, want)
	}
	if out.Len() != 0 {
		t.Errorf("wrote %q, want nothing", out.String())
	}
}
