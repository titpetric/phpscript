package runner

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner/coverage"
)

// coverageBlock finds the block starting at line in file, so a test asserts on
// position rather than on ordering.
func coverageBlock(t *testing.T, blocks []coverage.Block, file string, line int) coverage.Block {
	t.Helper()
	for _, b := range blocks {
		if b.File == file && b.StartLine == line {
			return b
		}
	}
	t.Fatalf("no block for %s:%d in %+v", file, line, blocks)
	return coverage.Block{}
}

// TestCoverageCounts covers the count semantics: a loop body is charged once
// per iteration, the loop header once, and an untaken branch stays at zero.
func TestCoverageCounts(t *testing.T) {
	src := `<?php
$total = 0;
for ($i = 0; $i < 3; $i++) {
	$total = $total + $i;
}
if ($total > 100) {
	echo "big";
}
echo $total;
`
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	rt := New(&out, Options{})
	rt.UpdateFilename("main.php")
	cov := coverage.New()
	rt.SetCoverage(cov)
	if err := rt.Run(prog); err != nil {
		t.Fatal(err)
	}
	if out.String() != "3" {
		t.Fatalf("output = %q", out.String())
	}

	blocks := cov.Blocks()
	if got := coverageBlock(t, blocks, "main.php", 2).Count; got != 1 {
		t.Errorf("assignment count = %d, want 1", got)
	}
	if got := coverageBlock(t, blocks, "main.php", 3); got.Count != 1 || got.EndLine != 3 {
		t.Errorf("for header = %+v, want count 1 clamped to its own line", got)
	}
	if got := coverageBlock(t, blocks, "main.php", 4).Count; got != 3 {
		t.Errorf("loop body count = %d, want 3", got)
	}
	if got := coverageBlock(t, blocks, "main.php", 7).Count; got != 0 {
		t.Errorf("untaken branch count = %d, want 0", got)
	}
}

// TestCoverageIncludedFiles covers the zero-count baseline: an included file
// registers every statement, so a function body the fixture never calls shows
// up at zero under the include's own filename.
func TestCoverageIncludedFiles(t *testing.T) {
	root := fstest.MapFS{
		"lib.php": &fstest.MapFile{Data: []byte(`<?php
function used()
{
	return "used";
}

function unused()
{
	return "unused";
}
`)},
	}
	prog, err := parser.Parse(`<?php
include "lib.php";
echo used();
`)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	rt := New(&out, Options{RootFS: root})
	rt.UpdateFilename("main.php")
	cov := coverage.New()
	rt.SetCoverage(cov)
	if err := rt.Run(prog); err != nil {
		t.Fatal(err)
	}
	if out.String() != "used" {
		t.Fatalf("output = %q", out.String())
	}

	blocks := cov.Blocks()
	if got := coverageBlock(t, blocks, "lib.php", 4).Count; got != 1 {
		t.Errorf("used() body count = %d, want 1", got)
	}
	if got := coverageBlock(t, blocks, "lib.php", 9).Count; got != 0 {
		t.Errorf("unused() body count = %d, want 0", got)
	}
	for _, b := range blocks {
		if b.File == "lib.php" && b.StartLine == 2 {
			t.Errorf("function declaration registered as a block: %+v", b)
		}
	}
}

// TestCoverageForcesInterpreter holds the flatstack boundary: a runtime created
// with NewFlatStack executes interpreted while a collector is installed, so the
// counts exist and no coverage support is needed in the bytecode backend.
func TestCoverageForcesInterpreter(t *testing.T) {
	prog, err := parser.Parse(`<?php
echo "covered";
`)
	if err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	rt := NewFlatStack(&out, Options{})
	rt.UpdateFilename("main.php")
	cov := coverage.New()
	rt.SetCoverage(cov)
	if err := rt.Run(prog); err != nil {
		t.Fatal(err)
	}
	if out.String() != "covered" {
		t.Fatalf("output = %q", out.String())
	}
	if got := coverageBlock(t, cov.Blocks(), "main.php", 2).Count; got != 1 {
		t.Errorf("echo count = %d, want 1", got)
	}
}
