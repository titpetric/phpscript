package list_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/list"
)

const apiSource = `<?php

namespace Task;

interface Finder {
	/**
	 * One workspace by folder, or null.
	 */
	function byFolder(string $folder): ?Model\Workspace;

	function all(): array;
}

class Workspace implements Finder {
	/**
	 * One workspace by folder, or null.
	 */
	public function byFolder(string $folder): ?Model\Workspace {
		return null;
	}

	/** Every workspace. */
	static function all(array $options = array(), $limit = 10): array {
		return array();
	}
}
`

// TestAPIReportsMethods covers the shape the flag exists for: one row per
// method, its declaration and the docblock above it, for classes and
// interfaces alike.
func TestAPIReportsMethods(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Workspace.php")
	if err := os.WriteFile(path, []byte(apiSource), 0o644); err != nil {
		t.Fatal(err)
	}

	rows, err := list.API([]string{dir})
	if err != nil {
		t.Fatalf("API: %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4: %#v", len(rows), rows)
	}
	want := []list.MethodRow{
		{Class: `Task\Finder`, Member: "byFolder", Signature: `byFolder(string $folder): ?Model\Workspace`, Doc: "One workspace by folder, or null."},
		{Class: `Task\Finder`, Member: "all", Signature: "all(): array"},
		{Class: `Task\Workspace`, Member: "byFolder", Signature: `byFolder(string $folder): ?Model\Workspace`, Doc: "One workspace by folder, or null."},
		{Class: `Task\Workspace`, Member: "all", Signature: "static all(array $options = array(), $limit = 10): array", Doc: "Every workspace."},
	}
	for i, w := range want {
		if rows[i] != w {
			t.Errorf("row %d = %#v, want %#v", i, rows[i], w)
		}
	}
}

// TestAPIMarkdownHeadings covers the table the command prints: the four
// columns of the issue, with an empty doc shown as <none>.
func TestAPIMarkdownHeadings(t *testing.T) {
	out := list.APIMarkdown([]list.MethodRow{
		{Class: `Task\Workspace`, Member: "byFolder", Signature: "byFolder(string $folder): ?Model\\Workspace", Doc: "One workspace by folder, or null."},
	})
	for _, want := range []string{"| Class", "| Member", "| Signature", "| Doc", "Task\\Workspace", "byFolder", "One workspace by folder, or null."} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not contain %q:\n%s", want, out)
		}
	}
}

// TestAPISkipsRefusedFile covers the fallback boundary: a file the parser
// refuses contributes no rows and the listing still succeeds.
func TestAPISkipsRefusedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.php"), []byte("<?php class {"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := list.API([]string{dir})
	if err != nil {
		t.Fatalf("API: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows from a refused file, want 0", len(rows))
	}
}
