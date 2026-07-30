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

	got, err := list.Expand([]string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || filepath.Base(got[0]) != "a.php" {
		t.Fatalf("dot = %v, want only a.php", got)
	}

	got, err = list.Expand([]string{"./..."})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("./... = %v, want 2 files", got)
	}
}

func TestExpandFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "only.php")
	mustWrite(t, path, "<?php\n")
	got, err := list.Expand([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %v", got)
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
