package lint

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// TestSetInclude covers what the flag buys and the ordering it carries. The
// linter runs the recorded path against the name registry before it checks any
// file and the names an application declares resolve as well as the standard
// library's. knownRuntime builds the registry once per process and a caller
// records the path before the first File.
func TestSetInclude(t *testing.T) {
	previous := includeFile
	t.Cleanup(func() { includeFile = previous })

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bootstrap.php"), []byte("<?php\nfunction site_helper() { return 1; }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	SetInclude("bootstrap.php")
	if includeFile != "bootstrap.php" {
		t.Fatalf("includeFile = %q, want the recorded path", includeFile)
	}

	rt := runner.New(io.Discard, runner.Options{SAPI: "cli", RootFS: os.DirFS(".")})
	stdlib.Register(rt)
	if rt.FunctionExists("site_helper") {
		t.Fatal("the name resolved before the include ran")
	}

	loadInclude(rt)
	if !rt.FunctionExists("site_helper") {
		t.Error("a name the include declared does not resolve after it ran")
	}
}

// TestSetIncludeIsQuietWhenItFails covers the refusal to fail a lint run over
// a bootstrap: a file that is not there, and one the parser rejects, each
// leave the registry alone and report nothing. An application whose
// bootstrap cannot run outside a request still gets the findings the standard
// library alone produces.
func TestSetIncludeIsQuietWhenItFails(t *testing.T) {
	previous := includeFile
	t.Cleanup(func() { includeFile = previous })

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.php"), []byte("<?php\nfunction broken( {\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	for _, path := range []string{"", "absent.php", "broken.php"} {
		SetInclude(path)
		rt := runner.New(io.Discard, runner.Options{SAPI: "cli", RootFS: os.DirFS(".")})
		stdlib.Register(rt)

		before, _ := rt.DefinedFunctions()
		loadInclude(rt)
		after, user := rt.DefinedFunctions()

		if len(before) != len(after) || len(user) != 0 {
			t.Errorf("include %q declared %d names and changed the internal table by %d", path, len(user), len(after)-len(before))
		}
	}
}
