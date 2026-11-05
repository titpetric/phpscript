package main

import (
	"fmt"
	"strings"
	"testing"
)

// TestLintOffenders pins every shape the rules refuse, in the order the
// findings sort: a parameter of the runtime type on a function, on a slice,
// and on a class method, and a narrowing in the binding and in a helper of its
// package.
func TestLintOffenders(t *testing.T) {
	t.Parallel()
	findings, err := lint("testdata/offenders")
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	want := []string{
		"offenders.go:14: count_array: parameter $array is *model.Array",
		"offenders.go:18: count_rows: parameter $rows is []*model.Array",
		"offenders.go:23: read_options: narrows options to *model.Array",
		"offenders.go:45: read_options_helper via readPath: narrows options to *model.Array",
		"offenders.go:69: Box::Fill: parameter $items is *model.Array",
	}
	if len(findings) != len(want) {
		t.Fatalf("got %d findings, want %d:\n%s", len(findings), len(want), join(findings))
	}
	for i, prefix := range want {
		if got := findings[i].String(); !strings.HasPrefix(got, prefix) {
			t.Errorf("finding %d is %q, want the prefix %q", i, got, prefix)
		}
	}
}

// TestLintClean covers the four reasons a narrowing has to exist: the
// collection helpers, the loud refusal the mutators document, a capability a
// Go-native value has not got, and a type switch with a case for the native
// shape. A registration whose name a table holds names no function to read and
// is no finding either.
func TestLintClean(t *testing.T) {
	t.Parallel()
	findings, err := lint("testdata/clean")
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("the compliant bindings reported %d findings:\n%s", len(findings), join(findings))
	}
}

// TestLintMissingRoot covers the walk failing: a path that is no directory is
// an error and never an empty report that reads as a pass.
func TestLintMissingRoot(t *testing.T) {
	t.Parallel()
	if _, err := lint("testdata/absent"); err == nil {
		t.Error("a missing root reported no error")
	}
}

// TestFindingString covers the position format, which an editor jumps to.
func TestFindingString(t *testing.T) {
	t.Parallel()
	f := finding{file: "stdlib/ps/core/platform.go", line: 333, binding: "strtr", message: "narrows pairs"}
	if got, want := f.String(), "stdlib/ps/core/platform.go:333: strtr: narrows pairs"; got != want {
		t.Errorf("String() is %q, want %q", got, want)
	}
}

func join(findings []finding) string {
	lines := make([]string, 0, len(findings))
	for _, f := range findings {
		lines = append(lines, fmt.Sprintf("  %s", f))
	}
	return strings.Join(lines, "\n")
}
