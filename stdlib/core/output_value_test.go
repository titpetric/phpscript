package core_test

import (
	"testing"
)

// TestVarDumpObject covers the visibility annotation, which is the one part of
// a dump that reads the class and not the value: a private property carries
// the declaring class, a protected one does not, and a public one carries
// nothing.
func TestVarDumpObject(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
class Point { public $x = 1; private $y = 2; protected $z = 3; }
var_dump(new Point());`)

	want := "object(Point)#1 (3) {\n" +
		"  [\"x\"]=>\n  int(1)\n" +
		"  [\"y\":\"Point\":private]=>\n  int(2)\n" +
		"  [\"z\":protected]=>\n  int(3)\n}\n"
	if got != want {
		t.Errorf("var_dump of an object =\n%q\nwant\n%q", got, want)
	}
}

// TestPrintRScalars covers what print_r makes of the three values that have no
// text: true is "1", and false and null are both the empty string, so a
// template printing one of them writes nothing, and never "false".
func TestPrintRScalars(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(print_r(true, true), print_r(false, true), print_r(null, true));`)

	want := "string(1) \"1\"\nstring(0) \"\"\nstring(0) \"\"\n"
	if got != want {
		t.Errorf("print_r of the empty scalars = %q, want %q", got, want)
	}
}
