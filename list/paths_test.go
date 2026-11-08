package list_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/titpetric/phpscript/list"
)

func TestExpandDotAndRecursive(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.php"), "<?php\n")
	mustWrite(t, filepath.Join(dir, "skip.txt"), "x")
	mustWrite(t, filepath.Join(dir, "sub", "b.php"), "<?php\n")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	got, err := list.ExpandFiles([]string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || filepath.Base(got[0]) != "a.php" {
		t.Fatalf("dot = %v, want only a.php", got)
	}

	got, err = list.ExpandFiles([]string{"./..."})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("./... = %v, want 2 files", got)
	}
}

func TestExpandFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "only.php")
	mustWrite(t, path, "<?php\n")
	got, err := list.ExpandFiles([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %v", got)
	}
}

// TestExpandSkipsTestdata covers the directory a recursive sweep leaves out.
// testdata holds the input a test feeds a tool, php a parser refuses among it,
// and a lint run over a project reports on the project. The path named
// outright still lists its files, as `go test` reads one.
func TestExpandSkipsTestdata(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "app.php"), "<?php\n")
	mustWrite(t, filepath.Join(dir, "testdata", "broken.php"), "<?php\n")
	mustWrite(t, filepath.Join(dir, "sub", "testdata", "broken.php"), "<?php\n")

	got, err := list.ExpandFiles([]string{dir + "/..."})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || filepath.Base(got[0]) != "app.php" {
		t.Fatalf("recursive = %v, want only app.php", got)
	}

	got, err = list.ExpandFiles([]string{filepath.Join(dir, "testdata") + "/..."})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || filepath.Base(got[0]) != "broken.php" {
		t.Fatalf("testdata named outright = %v, want broken.php", got)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
