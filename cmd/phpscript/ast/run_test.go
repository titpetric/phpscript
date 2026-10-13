package ast

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNewCommand covers the command every other phpscript command is built
// alongside: the constructor takes the configuration and the global options
// because one signature covers all nine, and this command reads neither, so
// both arguments are nil here and the command still builds. It declares no
// flags of its own, so Bind stays nil and the shared flags are all it binds.
func TestNewCommand(t *testing.T) {
	command := NewCommand(nil, nil)

	if command.Name != "ast" {
		t.Errorf("Name = %q, want %q", command.Name, "ast")
	}
	if command.Title != Name {
		t.Errorf("Title = %q, want %q", command.Title, Name)
	}
	if command.Bind != nil {
		t.Error("Bind is set, want nil: the ast command declares no flags of its own")
	}
	if command.Run == nil {
		t.Fatal("Run is nil")
	}
}

// TestRun covers the token stream `phpscript ast` prints: one line per token
// with the line number, the PHP token name and the source text, and a CHAR
// line for a single-character token, which token_get_all reports as a bare
// string with no id to name. A call with no file is a usage error, because the
// command reads a path and never standard input.
func TestRun(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "echo.php")
	if err := os.WriteFile(script, []byte("<?php\necho 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := Run([]string{script}); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	for _, want := range []string{"T_OPEN_TAG", "T_ECHO", "T_LNUMBER", "CHAR"} {
		if !strings.Contains(out, want) {
			t.Errorf("token stream does not name %q:\n%s", want, out)
		}
	}
	// The echo sits on the second line of the file, and the line number is
	// what makes a token stream readable against the source.
	if !strings.Contains(out, `T_ECHO                        "echo"`) {
		t.Errorf("the echo token is not rendered with its text:\n%s", out)
	}
	if !strings.Contains(out, `   2  T_ECHO`) {
		t.Errorf("the echo token does not carry line 2:\n%s", out)
	}

	if err := Run(nil); err == nil {
		t.Error("Run(nil) = nil, want the usage error")
	}
	if _, err := os.Stat(filepath.Join(dir, "missing.php")); err == nil {
		t.Fatal("the missing file exists")
	}
	if err := Run([]string{filepath.Join(dir, "missing.php")}); err == nil {
		t.Error("Run over a file that is not there = nil, want the read error")
	}
}

// captureStdout runs fn with os.Stdout on a pipe and returns what it wrote.
// Run prints through the package-level fmt, so the file descriptor is the only
// place to read its output from.
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
