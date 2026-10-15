package list

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/cmd/phpscript/helpdocs"
)

// TestRunListsPaths covers the default behaviour: a markdown table of the PHP
// files a path selects.
func TestRunListsPaths(t *testing.T) {
	t.Parallel()
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

// TestRunListsBindings covers --bindings: the registered surface of this
// build, with no tree involved.
func TestRunListsBindings(t *testing.T) {
	t.Parallel()
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

// TestNewCommand covers the one flag this command owns. --bindings switches
// the subject from a tree to the binary, so it defaults off: a bare `phpscript
// list` scans paths. The constructor takes the configuration and the global
// options because one signature covers all nine commands, and this command
// reads neither, so both arguments are nil here.
func TestNewCommand(t *testing.T) {
	t.Parallel()
	command := NewCommand(nil, nil)

	if command.Name != "list" {
		t.Errorf("Name = %q, want %q", command.Name, "list")
	}
	if command.Title != Name {
		t.Errorf("Title = %q, want %q", command.Title, Name)
	}
	if command.Bind == nil {
		t.Fatal("Bind is nil, want the --bindings flag")
	}

	set := helpdocs.NewFlagSet("list")
	command.Bind(set)
	flag := set.Lookup("bindings")
	if flag == nil {
		t.Fatal("--bindings is not bound")
	}
	if flag.DefValue != "false" {
		t.Errorf("--bindings default = %q, want false: a bare list scans paths", flag.DefValue)
	}
}

// TestRun covers the exported entry point, which is run() pointed at stdout.
// The markdown table is the same one a redirected run writes, so the one
// assertion worth making here is that os.Stdout is where it lands.
func TestRun(t *testing.T) {
	dir := t.TempDir()
	source := "<?php\n// @route GET /hello\nclass Greeter {}\n"
	if err := os.WriteFile(filepath.Join(dir, "hello.php"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	saved := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	runErr := Run([]string{dir}, false)
	writer.Close()
	os.Stdout = saved
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}

	var out bytes.Buffer
	if _, err := io.Copy(&out, reader); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"GET /hello", "hello.php", "Greeter"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout does not contain %q:\n%s", want, out.String())
		}
	}
}

// TestRunRejectsBindingsWithPaths pins the refusal. The two modes answer about
// different things, so a caller that named both named one of them by mistake and is
// told which one it drops, and never left to guess.
func TestRunRejectsBindingsWithPaths(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	err := run(&out, []string{"."}, true)
	if err == nil {
		t.Fatal("run returned nil error for --bindings with a path argument")
	}
	if want := "list --bindings takes no path arguments"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err, want)
	}
	if out.Len() != 0 {
		t.Errorf("wrote %q, want nothing", out.String())
	}
}
