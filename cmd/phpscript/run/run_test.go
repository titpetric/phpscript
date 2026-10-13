package run

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/titpetric/phpscript/config"
	"github.com/titpetric/phpscript/internal/flags"
)

// TestNewCommand covers what the constructor closes over. The configuration
// arrives as a pointer so one signature covers all nine commands, and this one
// reads it, so a nil pointer has to resolve to the zero configuration before
// the Run closure captures it. The command declares no flags of its own: a run
// takes a script path and the shared flags.
func TestNewCommand(t *testing.T) {
	command := NewCommand(nil, &flags.Options{})

	if command.Name != "run" {
		t.Errorf("Name = %q, want %q", command.Name, "run")
	}
	if command.Title != Name {
		t.Errorf("Title = %q, want %q", command.Title, Name)
	}
	if command.Bind != nil {
		t.Error("Bind is set, want nil: run declares no flags of its own")
	}
	if err := command.Run(context.Background(), nil); err == nil {
		t.Error("Run with no arguments = nil, want the usage error")
	}
}

// TestRun covers one generation of the loop: the script is read, the runtime
// is built around it and its output reaches os.Stdout. The run returns once
// the script ends, because nothing sent SIGHUP and no reload is pending.
func TestRun(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.php"), []byte("<?php echo \"from the script\\n\";\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	saved := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	runErr := Run(context.Background(), []string{"hello.php"}, config.Value(nil), &flags.Options{})
	writer.Close()
	os.Stdout = saved
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}

	var out bytes.Buffer
	if _, err := io.Copy(&out, reader); err != nil {
		t.Fatal(err)
	}
	if out.String() != "from the script\n" {
		t.Errorf("script output = %q, want %q", out.String(), "from the script\n")
	}

	if err := Run(context.Background(), nil, config.Value(nil), &flags.Options{}); err == nil {
		t.Error("Run with no arguments = nil, want the usage error")
	}
}

// TestResolveEntrypoint pins which scripts re-root the runtime. A path the
// working directory can name keeps it as the root, because PHP resolves an
// include against the working directory; anything the working directory cannot
// name has to root at the script instead, or the leading slash is stripped and
// the file is looked for in the wrong place.
func TestResolveEntrypoint(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	outside := filepath.Join(t.TempDir(), "other.php")

	cases := []struct {
		name       string
		arg        string
		wantScript string
		wantRoot   string
	}{
		{"relative", "sub/x.php", "sub/x.php", "."},
		{"relative dot", "./x.php", "x.php", "."},
		{"absolute inside", filepath.Join(dir, "x.php"), "x.php", "."},
		{"absolute outside", outside, "other.php", filepath.Dir(outside)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			script, root, err := resolveEntrypoint(c.arg)
			if err != nil {
				t.Fatalf("resolveEntrypoint(%q): %v", c.arg, err)
			}
			if script != c.wantScript || root != c.wantRoot {
				t.Fatalf("resolveEntrypoint(%q) = %q, %q, want %q, %q",
					c.arg, script, root, c.wantScript, c.wantRoot)
			}
		})
	}
}

// TestResolveEntrypointClimbingOut covers the sibling directory case, which
// looks relative but the working directory still cannot name it.
func TestResolveEntrypointClimbingOut(t *testing.T) {
	dir := t.TempDir()
	here := filepath.Join(dir, "here")
	there := filepath.Join(dir, "there")
	for _, d := range []string{here, there} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(here)

	script, root, err := resolveEntrypoint("../there/x.php")
	if err != nil {
		t.Fatal(err)
	}
	if script != "x.php" || root != there {
		t.Fatalf("resolveEntrypoint = %q, %q, want %q, %q", script, root, "x.php", there)
	}
}
