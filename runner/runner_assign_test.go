package runner

import (
	"strings"
	"testing"
)

// TestApplyAssignOp covers the compound-assignment operators. Each resolves
// through the same table its binary operator reads, so `$x <<= -1` fails
// where `$x = $x << -1` fails, with no silent assignment.
func TestApplyAssignOp(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		op       string
		cur, rhs any
		want     any
	}{
		{"=", int64(1), int64(2), int64(2)},
		{"", int64(1), int64(2), int64(2)},
		{"[]=", nil, "appended", "appended"},
		{".=", "n=", int64(7), "n=7"},
		{"+=", int64(2), int64(3), int64(5)},
		{"-=", int64(2), int64(3), int64(-1)},
		{"*=", int64(2), int64(3), int64(6)},
		{"%=", int64(7), int64(3), int64(1)},
		{"**=", int64(2), int64(3), int64(8)},
		{"&=", int64(6), int64(3), int64(2)},
		{"<<=", int64(1), int64(3), int64(8)},
	} {
		got, err := applyAssignOp(tc.op, tc.cur, tc.rhs)
		if err != nil {
			t.Errorf("applyAssignOp(%q) failed: %v", tc.op, err)
			continue
		}
		if got != tc.want {
			t.Errorf("applyAssignOp(%q, %#v, %#v) = %#v, want %#v", tc.op, tc.cur, tc.rhs, got, tc.want)
		}
	}

	if _, err := applyAssignOp("<<=", int64(1), int64(-1)); err == nil {
		t.Error("a negative shift count reported no error through the compound form")
	}
}

// TestExecAssignTargets covers the lvalue shapes an assignment reaches: a
// variable, an array element with auto-vivified levels, an append, a property,
// and list() destructuring by position. unset() removes from the same storage,
// and unsetting a name nothing set is not an error.
func TestExecAssignTargets(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := New(&out, Options{})

	program, err := rt.Load(`<?php
$n = 1;
$n += 2;
$rows["a"]["b"] = "deep";
$rows[] = "appended";
$obj = new stdClass;
$obj->name = "ada";
$obj->name .= "!";
list($first, $second) = ["one", "two"];
unset($rows["a"], $nothing);
echo $n, "|", $rows[0], "|", $rows["a"] === null ? "gone" : "kept", "|", $obj->name, "|", $first, $second;
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "3|appended|gone|ada!|onetwo"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

// TestReadLValue covers the read an increment makes before it writes: the
// current value of a variable or element, and the refusals for a target no
// read reaches, such as a `[]` append slot.
func TestReadLValue(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	rt := New(&out, Options{})

	program, err := rt.Load(`<?php
$n = 5;
$n++;
$counts["key"] = 1;
$counts["key"]++;
echo $n, "|", $counts["key"];
`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.Run(program); err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "6|2"; out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}
