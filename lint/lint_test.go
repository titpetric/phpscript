package lint_test

import (
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
