package core_test

import (
	"testing"
)

// TestVarExport covers the source var_export writes, which has to read back as
// the value behind it. The single-quoted string escapes the quote and the
// backslash alone, and a whole float keeps its ".0", because
// var_export(1.0) writing "1" would read back as an int.
//
// A nested array breaks after the arrow and indents two and the inner "array ("
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
// var_export answers the source and writes nothing and a caller nests one
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
// makes string(6) out of five characters and a script sees the encoding it
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

// TestPrintR covers the shape print_r writes for a reader: eight columns per
// level of nesting with the entries four past the parenthesis, and the blank
// line php leaves after a nested array.
//
// A bare scalar has no lines of its own, and the float carries precision=14
// digits where var_dump passes the round-tripping ones and 0.1+0.2 prints 0.3
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
