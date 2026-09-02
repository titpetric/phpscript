package coverage_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/runner/coverage"
)

var profileBlocks = []coverage.Block{
	{File: "app.php", StartLine: 3, EndLine: 3, NumStmt: 1, Count: 0},
	{File: "app.php", StartLine: 2, EndLine: 2, NumStmt: 1, Count: 4},
	{File: "lib.php", StartLine: 2, EndLine: 2, NumStmt: 2, Count: 1},
}

var profileSource = map[string]string{
	"app.php": "<?php\n  echo \"a\";\n  echo \"b\";\n",
	"lib.php": "<?php\nfunction f() { return 1; }\n",
}

func testSource(file string) []string {
	src, ok := profileSource[file]
	if !ok {
		return nil
	}
	return strings.Split(src, "\n")
}

// TestColumns covers the half the collector cannot answer: it records line
// ranges, and a profile carries columns spanning the statement text rather than
// the indentation around it.
func TestColumns(t *testing.T) {
	blocks := coverage.Columns(profileBlocks, testSource)
	if len(blocks) != 3 {
		t.Fatalf("blocks = %d, want 3", len(blocks))
	}
	// Sorted by file, then position, which is the order a profile is written.
	if blocks[0].File != "app.php" || blocks[0].StartLine != 2 {
		t.Errorf("first block = %+v, want app.php line 2", blocks[0])
	}
	// `  echo "a";` is indented two spaces and eleven characters long.
	if blocks[0].StartCol != 3 || blocks[0].EndCol != 12 {
		t.Errorf("columns = %d..%d, want 3..12", blocks[0].StartCol, blocks[0].EndCol)
	}

	t.Run("unreadable source", func(t *testing.T) {
		blocks := coverage.Columns(profileBlocks, func(string) []string { return nil })
		for _, block := range blocks {
			if block.StartCol != 1 || block.EndCol != 1 {
				t.Errorf("block %+v, want column 1 on both ends", block)
			}
		}
	})
}

// TestWriteProfile pins the format, which is the one go test writes and go tool
// cover reads.
func TestWriteProfile(t *testing.T) {
	var out strings.Builder
	if err := coverage.WriteProfile(&out, coverage.Columns(profileBlocks, testSource)); err != nil {
		t.Fatalf("WriteProfile: %v", err)
	}
	want := "mode: count\n" +
		"app.php:2.3,2.12 1 4\n" +
		"app.php:3.3,3.12 1 0\n" +
		"lib.php:2.1,2.27 2 1\n"
	if out.String() != want {
		t.Errorf("profile =\n%s\nwant\n%s", out.String(), want)
	}
}

// TestPercent covers the statement-weighted percentage go test reports.
func TestPercent(t *testing.T) {
	blocks := coverage.Columns(profileBlocks, testSource)
	// Three of four statements ran: app.php line 2 and lib.php's pair.
	if got := coverage.Percent(blocks); got != 75 {
		t.Errorf("Percent = %v, want 75", got)
	}
	if got := coverage.Percent(nil); got != 0 {
		t.Errorf("Percent(nil) = %v, want 0", got)
	}
}
