package lint_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/titpetric/phpscript/lint"
)

func TestFlatstackLinterCompatibility(t *testing.T) {
	compatibleSrc := `<?php
	$a = 1 + 2;
	echo $a;
	?>`

	diag, err := lint.FlatstackFile("compatible.php", compatibleSrc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diag.Message != "[flatstack compatible] 100% compatible with flatstack bytecode engine" {
		t.Fatalf("expected compatible message, got %s", diag.Message)
	}

	unsupportedSrc := `<?php
	class Foo extends Bar {
		public function baz() {}
	}
	?>`

	diag, err = lint.FlatstackFile("unsupported.php", unsupportedSrc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diag.Message == "[flatstack compatible] 100% compatible with flatstack bytecode engine" {
		t.Fatalf("expected unsupported diagnostic for class inheritance, got %s", diag.Message)
	}
}

func TestPathsReportsParseErrorsAndContinues(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"invalid.php": `<?php class Foo extends Bar {}`,
		"lint.php":    `<?php if ($value = true) {}`,
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	diags, err := lint.Paths([]string{dir})
	if err != nil {
		t.Fatalf("Paths returned a parser error instead of continuing: %v", err)
	}
	if len(diags) != 2 {
		t.Fatalf("got %d diagnostics, want parser and lint diagnostics: %+v", len(diags), diags)
	}
	if got := diags[0].Message; len(got) < len("parse error:") || got[:len("parse error:")] != "parse error:" {
		t.Fatalf("first diagnostic = %q, want parse error", got)
	}
	if got := diags[1].Message; got != "assignment in conditional statement" {
		t.Fatalf("second diagnostic = %q, want assignment diagnostic", got)
	}
}

func TestPathsLintsPHPSectionOfPHPTFixture(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "assignment.phpt")
	src := `name: assignment condition
description: Lint the PHP section only.
---
<?php
if ($value = true) {}
---
done
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	diags, err := lint.Paths([]string{path})
	if err != nil {
		t.Fatalf("Paths returned an error: %v", err)
	}
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want assignment diagnostic: %+v", len(diags), diags)
	}
	if got := diags[0].Message; got != "assignment in conditional statement" {
		t.Fatalf("diagnostic = %q, want assignment diagnostic", got)
	}
	if got := diags[0].Line; got != 5 {
		t.Fatalf("diagnostic line = %d, want physical fixture line 5", got)
	}
}

func TestFileReportsChainedAssignment(t *testing.T) {
	src := `<?php
$inlines = $blocks = array();
$one = array();
$a = $b = $c = 1;
$paren = ($x = 5);
$obj->left = $obj->right = "v";
function scoped() { $p = $q = array(); }
class Holder { function method() { $r = $s = 0; } }
foreach (array(1) as $v) { $t = $u = 1; }
for ($i = 0; $i < 1; $i++) { $w = $y = 1; }
if (true) { $m = $n = 1; }
`
	diags, err := lint.File("chain.php", src)
	if err != nil {
		t.Fatalf("File returned an error: %v", err)
	}

	// Every chained statement is reported once, whatever scope it sits in, and
	// a chain of three names is still one finding rather than one per link.
	wantLines := []int{2, 4, 5, 6, 7, 8, 9, 10, 11}
	if len(diags) != len(wantLines) {
		t.Fatalf("got %d diagnostics, want %d: %+v", len(diags), len(wantLines), diags)
	}
	for i, want := range wantLines {
		if diags[i].Line != want {
			t.Errorf("diagnostic %d line = %d, want %d (%q)", i, diags[i].Line, want, diags[i].Message)
		}
		if got := diags[i].Message; got != "chained assignment binds one value to several names" {
			t.Errorf("diagnostic %d message = %q", i, got)
		}
	}
}

func TestFileAcceptsSingleAssignment(t *testing.T) {
	src := `<?php
$value = array();
$sum = 1 + 2;
$result = compute($value);
$value[] = $sum;
$value["k"] = $sum;
`
	diags, err := lint.File("single.php", src)
	if err != nil {
		t.Fatalf("File returned an error: %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("got %d diagnostics for unchained assignments: %+v", len(diags), diags)
	}
}
