package core_test

import (
	"testing"
)

// TestVarExport covers the source var_export writes, which has to read back as
// the value behind it. The single-quoted string escapes the quote and the
// backslash and nothing else, and a whole float keeps its ".0", because
// var_export(1.0) writing "1" would read back as an int.
//
// A nested array breaks after the arrow and indents two, so the inner "array ("
// sits on its own line under the key.
func TestVarExport(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_export([1, 2, 3]);
echo "\n";
var_export(['a' => 1, 'b' => [2, 3]]);
echo "\n";
var_export("it's");
echo "\n";
var_export(1.0);
echo "\n";
var_export([true, false, null, 0.1]);
echo "\n";`)

	want := "array (\n  0 => 1,\n  1 => 2,\n  2 => 3,\n)\n" +
		"array (\n  'a' => 1,\n  'b' => \n  array (\n    0 => 2,\n    1 => 3,\n  ),\n)\n" +
		"'it\\'s'\n" +
		"1.0\n" +
		"array (\n  0 => true,\n  1 => false,\n  2 => NULL,\n  3 => 0.1,\n)\n"
	if got != want {
		t.Errorf("var_export =\n%q\nwant\n%q", got, want)
	}
}

// TestVarExportReturn covers the second argument: with $return true
// var_export answers the source and writes nothing, so a caller nests one
// export inside another string.
func TestVarExportReturn(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
$source = var_export([1], true);
echo "[", $source, "]";`)

	want := "[array (\n  0 => 1,\n)]"
	if got != want {
		t.Errorf("var_export with return = %q, want %q", got, want)
	}
}

// TestVarDump covers the annotation var_dump carries: the type, the element
// count of a composite, and the length of a string in bytes. A two-byte é
// makes string(6) out of five characters, so a script sees the encoding it
// actually holds.
func TestVarDump(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump("héllo");
var_dump(0.1 + 0.2);
var_dump([1 => "a", "k" => [true]]);`)

	want := "string(6) \"héllo\"\n" +
		"float(0.30000000000000004)\n" +
		"array(2) {\n  [1]=>\n  string(1) \"a\"\n  [\"k\"]=>\n  array(1) {\n    [0]=>\n    bool(true)\n  }\n}\n"
	if got != want {
		t.Errorf("var_dump =\n%q\nwant\n%q", got, want)
	}
}

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

// TestPrintR covers the shape print_r writes for a reader: eight columns per
// level of nesting with the entries four past the parenthesis, and the blank
// line php leaves after a nested array.
//
// A bare scalar has no lines of its own, and the float carries precision=14
// digits where var_dump passes the round-tripping ones, so 0.1+0.2 prints 0.3
// here and 0.30000000000000004 there.
func TestPrintR(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
print_r([1, 2]);
print_r(['a' => ['b' => 1]]);
echo print_r("plain", true), "\n";
echo print_r(0.1 + 0.2, true), "\n";`)

	want := "Array\n(\n    [0] => 1\n    [1] => 2\n)\n" +
		"Array\n(\n    [a] => Array\n        (\n            [b] => 1\n        )\n\n)\n" +
		"plain\n" +
		"0.3\n"
	if got != want {
		t.Errorf("print_r =\n%q\nwant\n%q", got, want)
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

// TestFloatFormatting covers where php switches a float to the exponent form,
// which the decimal point decides and not Go's 'g' thresholds: 1e16 is still
// written out at seventeen digits and 1e17 is not, and a small value turns at
// the third leading zero. The mantissa always carries a fraction and the
// exponent no leading zero, so the spelling is 1.0E+17 and not 1e+17.
//
// echo passes precision=14 where var_dump passes the round-tripping form, so
// one float has two spellings: 1/3 is 0.33333333333333 to echo.
func TestFloatFormatting(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(1e15, 1e16, 1e17, 1e-4, 1e-5, 1.0E+100, -0.0);
echo 1e16, " ", 0.1 + 0.2, " ", 1/3, "\n";`)

	want := "float(1000000000000000)\nfloat(10000000000000000)\nfloat(1.0E+17)\n" +
		"float(0.0001)\nfloat(1.0E-5)\nfloat(1.0E+100)\nfloat(-0)\n" +
		"1.0E+16 0.3 0.33333333333333\n"
	if got != want {
		t.Errorf("float formatting =\n%q\nwant\n%q", got, want)
	}
}

// TestFloatNonFinite covers the three values that are not numbers, which print
// as names in every renderer and carry no sign in var_export.
func TestFloatNonFinite(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_export(INF);
echo "\n";
var_dump(INF, -INF, NAN);`)

	want := "INF\nfloat(INF)\nfloat(-INF)\nfloat(NAN)\n"
	if got != want {
		t.Errorf("non-finite floats = %q, want %q", got, want)
	}
}
