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
