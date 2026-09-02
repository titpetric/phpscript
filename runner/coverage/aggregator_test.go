package coverage_test

import (
	"testing"

	"github.com/titpetric/phpscript/runner/coverage"
)

const aggregatorSource = `<?php
$a = 1;
echo $a;
function helper() { return 2; }
`

// TestAggregator_Add covers the fold every request goes through: a statement
// two runs reached is one entry whose counts add.
func TestAggregator_Add(t *testing.T) {
	a := coverage.NewAggregator()
	a.Add(collect(t, "app.php", aggregatorSource, 1))
	a.Add(collect(t, "app.php", aggregatorSource, 2))

	for _, block := range a.Blocks() {
		if block.Count != 3 {
			t.Errorf("line %d count = %d, want 1 + 2", block.StartLine, block.Count)
		}
	}
	if files := a.Files(); len(files) != 1 || files[0] != "app.php" {
		t.Errorf("files = %v, want app.php once", files)
	}
}

// TestAggregator_Blocks covers the entries a profile is written from: one per
// executable statement, in the order the profile carries them.
func TestAggregator_Blocks(t *testing.T) {
	a := coverage.NewAggregator()
	a.Add(collect(t, "two.php", aggregatorSource, 1))
	a.Add(collect(t, "one.php", aggregatorSource, 1))

	// Three per file: the assignment, the echo, and the return inside
	// helper(). The declaration itself is not executable and registers nothing.
	blocks := a.Blocks()
	if len(blocks) != 6 {
		t.Fatalf("blocks = %+v, want three per file", blocks)
	}
	for i, block := range blocks {
		if block.NumStmt != 1 {
			t.Errorf("line %d numstmt = %d, want 1", block.StartLine, block.NumStmt)
		}
		if i > 0 && blocks[i-1].File > block.File {
			t.Errorf("blocks are not sorted by file: %v", blocks)
		}
		if i > 0 && blocks[i-1].File == block.File && blocks[i-1].StartLine > block.StartLine {
			t.Errorf("blocks are not sorted by position: %v", blocks)
		}
	}
}

// TestAggregator_Functions covers the declaration spans a per-function report
// is charged against, recorded once however many collectors named them.
func TestAggregator_Functions(t *testing.T) {
	a := coverage.NewAggregator()
	a.Add(collect(t, "app.php", aggregatorSource, 1))
	a.Add(collect(t, "app.php", aggregatorSource, 1))

	funcs := a.Functions()
	if len(funcs) != 1 || funcs[0].Name != "helper" || funcs[0].File != "app.php" {
		t.Fatalf("functions = %+v, want helper once", funcs)
	}
	if funcs[0].StartLine != 4 || funcs[0].EndLine < funcs[0].StartLine {
		t.Errorf("helper span = %d..%d, want the declaration line", funcs[0].StartLine, funcs[0].EndLine)
	}
}

// TestAggregator_Empty covers the question the shutdown flush asks: a server
// nothing reached writes no profile, because an empty one reads as a
// measurement that found nothing.
func TestAggregator_Empty(t *testing.T) {
	a := coverage.NewAggregator()
	if !a.Empty() {
		t.Fatal("a new aggregator is not empty")
	}
	a.Add(collect(t, "app.php", aggregatorSource, 0))
	if a.Empty() {
		t.Error("an aggregator holding a registered file reports empty")
	}
}

// TestAggregator is the reason the aggregator exists: a collector
// keys statements by AST node, so a second parse of the same source produces a
// second set of keys. The aggregator keys them by what the profile will say, so
// re-parsing adds counts rather than entries.
func TestAggregator(t *testing.T) {
	a := coverage.NewAggregator()
	for range 10 {
		a.Add(collect(t, "app.php", aggregatorSource, 1))
	}
	blocks := a.Blocks()
	if len(blocks) != 3 {
		t.Fatalf("blocks = %d after ten parses, want 3", len(blocks))
	}
	for _, block := range blocks {
		if block.Count != 10 {
			t.Errorf("line %d count = %d, want 10", block.StartLine, block.Count)
		}
	}
}

// TestAggregator_Files pins that the composed key carries the filename,
// so the same line of two files stays two entries.
func TestAggregator_Files(t *testing.T) {
	a := coverage.NewAggregator()
	a.Add(collect(t, "one.php", aggregatorSource, 1))
	a.Add(collect(t, "two.php", aggregatorSource, 1))

	if got := len(a.Blocks()); got != 6 {
		t.Errorf("blocks = %d, want three per file", got)
	}
	files := a.Files()
	if len(files) != 2 || files[0] != "one.php" || files[1] != "two.php" {
		t.Errorf("files = %v, want both, sorted", files)
	}
}

// TestNewAggregator pins that a command with coverage off costs nothing.
func TestNewAggregator(t *testing.T) {
	a := coverage.NewAggregator()
	a.Add(nil)
	if !a.Empty() {
		t.Error("a nil collector folded something in")
	}
}
