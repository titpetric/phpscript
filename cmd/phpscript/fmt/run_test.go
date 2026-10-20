package fmt

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// TestNewCommand covers the one flag this command owns. -l is the pipeline
// form: it reports what a rewrite would touch and writes nothing, so a job can
// fail on an unformatted tree without changing it. The constructor takes the
// configuration and the global options because one signature covers all nine
// commands, and this command reads neither, so both arguments are nil here.
func TestNewCommand(t *testing.T) {
	t.Parallel()
	command := NewCommand(nil, nil)

	if command.Name != "fmt" {
		t.Errorf("Name = %q, want %q", command.Name, "fmt")
	}
	if command.Title != Name {
		t.Errorf("Title = %q, want %q", command.Title, Name)
	}
	if command.Bind == nil {
		t.Fatal("Bind is nil, want the --list flag")
	}

	flags := pflag.NewFlagSet("fmt", pflag.ContinueOnError)
	command.Bind(flags)

	list := flags.Lookup("list")
	if list == nil {
		t.Fatal("--list is not bound")
	}
	if list.Shorthand != "l" {
		t.Errorf("--list shorthand = %q, want %q", list.Shorthand, "l")
	}
	if list.DefValue != "false" {
		t.Errorf("--list default = %q, want false: a bare fmt rewrites", list.DefValue)
	}
}

// TestRun covers both halves of the command over the same file: the default
// rewrites it in place and names it on stdout, and -l names it and leaves the
// bytes alone. The test writes spaces for indentation and no trailing
// newline, both of which the formatter normalises, so the file needs
// formatting by construction.
func TestRun(t *testing.T) {
	const unformatted = "<?php\nif (true) {\n  echo 1;\n}"

	for _, test := range []struct {
		name        string
		list        bool
		wantRewrite bool
	}{
		{name: "rewrite", wantRewrite: true},
		{name: "list", list: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			script := filepath.Join(t.TempDir(), "index.php")
			if err := os.WriteFile(script, []byte(unformatted), 0o644); err != nil {
				t.Fatal(err)
			}

			out := captureStdout(t, func() {
				if err := Run([]string{script}, test.list); err != nil {
					t.Fatalf("Run: %v", err)
				}
			})
			if !strings.Contains(out, script) {
				t.Errorf("stdout does not name the file:\n%s", out)
			}

			after, err := os.ReadFile(script)
			if err != nil {
				t.Fatal(err)
			}
			rewritten := string(after) != unformatted
			if rewritten != test.wantRewrite {
				t.Errorf("file rewritten = %t, want %t:\n%q", rewritten, test.wantRewrite, after)
			}
			if test.wantRewrite && !strings.HasSuffix(string(after), "}\n") {
				t.Errorf("formatted file does not end in a newline:\n%q", after)
			}
		})
	}
}

// TestRunReportsSkippedFiles covers the file the formatter refuses, PHP
// phpscript cannot parse: run names it on stderr and leaves its bytes alone,
// while the rest of the run continues. A directory of PHP holds both kinds.
func TestRunReportsSkippedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken.php")
	if err := os.WriteFile(broken, []byte("<?php class Foo {"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ok.php"), []byte("<?php\nif (true) {\n  echo 1;\n}"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if err := run([]string{dir}, false, &out, &errOut); err != nil {
		t.Fatalf("run: %v", err)
	}

	if !strings.Contains(errOut.String(), "skipped "+broken) {
		t.Errorf("stderr does not report the skipped file:\n%s", errOut.String())
	}
	if !strings.Contains(out.String(), filepath.Join(dir, "ok.php")) {
		t.Errorf("the file beside it was not formatted:\n%s", out.String())
	}
	after, err := os.ReadFile(broken)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != "<?php class Foo {" {
		t.Errorf("the skipped file was rewritten:\n%q", after)
	}
}

// captureStdout runs fn with os.Stdout on a pipe and returns what it wrote.
// Run writes to os.Stdout directly, so the file descriptor is the only place
// to read its output from.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	saved := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	fn()
	writer.Close()
	os.Stdout = saved

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
