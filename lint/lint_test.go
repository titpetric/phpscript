package lint_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/titpetric/phpscript/lint"
)

func TestAssignmentInConditionalStatement(t *testing.T) {
	diags, err := lint.File("test.php", `<?php
$foo = false;
if (!$foo) { echo "ok"; }
if ($row = fn()) { echo "bad"; }
if (($next = fn()) !== false) { echo "nested"; }
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 2 {
		t.Fatalf("got %d diagnostics, want 2: %+v", len(diags), diags)
	}
	for _, d := range diags {
		if d.Message != "assignment in conditional statement" {
			t.Fatalf("unexpected diagnostic: %+v", d)
		}
	}
}

func TestPathsUsesRecursiveListPattern(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "top.php"), "<?php\nif ($top = true) {}\n")
	mustWrite(t, filepath.Join(dir, "nested", "nested.php"), "<?php\nif ($nested = true) {}\n")

	diags, err := lint.Paths([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 1 || filepath.Base(diags[0].File) != "top.php" {
		t.Fatalf("directory diagnostics = %+v, want only top.php", diags)
	}

	diags, err = lint.Paths([]string{dir + "/..."})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 2 {
		t.Fatalf("recursive diagnostics = %+v, want 2", diags)
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
