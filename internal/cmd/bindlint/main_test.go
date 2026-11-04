package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRunTree is the gate: no binding in this module points at a runtime value
// type. A new one fails here and in `atkins lint` both.
func TestRunTree(t *testing.T) {
	t.Parallel()
	var out, errs bytes.Buffer
	if code := run([]string{"../../.."}, &out, &errs); code != 0 {
		t.Errorf("the tree reports %d findings:\n%s", code, out.String())
	}
	if errs.Len() != 0 {
		t.Errorf("stderr: %s", errs.String())
	}
}

// TestRunFindings covers the exit codes the pipeline reads: 1 with findings, 2
// when the walk fails, and the findings go to stdout and the failure to
// stderr.
func TestRunFindings(t *testing.T) {
	t.Parallel()
	var out, errs bytes.Buffer
	if code := run([]string{"testdata/offenders"}, &out, &errs); code != 1 {
		t.Errorf("exit code is %d, want 1", code)
	}
	if !strings.Contains(out.String(), "count_array: parameter $array is *model.Array") {
		t.Errorf("stdout names no finding:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "5 bindings point at a runtime value type") {
		t.Errorf("stdout carries no total:\n%s", out.String())
	}

	out.Reset()
	if code := run([]string{"testdata/absent"}, &out, &errs); code != 2 {
		t.Errorf("exit code for a missing root is %d, want 2", code)
	}
	if !strings.HasPrefix(errs.String(), "bindlint:") {
		t.Errorf("stderr is %q, want the command's name ahead of the error", errs.String())
	}
}

// TestRunDefaultRoot covers the argument-free invocation, which reads the
// working directory: the package's own directory holds no binding.
func TestRunDefaultRoot(t *testing.T) {
	t.Parallel()
	var out, errs bytes.Buffer
	if code := run(nil, &out, &errs); code != 0 {
		t.Errorf("exit code is %d, want 0:\n%s%s", code, out.String(), errs.String())
	}
}
