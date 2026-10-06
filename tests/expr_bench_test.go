package tests_test

import (
	"testing"
)

// BenchmarkScriptExprHeavy is the end-to-end companion to the runner expr
// benchmarks: a loop body that is almost entirely expression evaluation
// (arithmetic, comparison, ternary, concat, a binding call), so the number
// moves with the expression engine rather than with any single binding.
func BenchmarkScriptExprHeavy(b *testing.B) {
	benchmarkScript(b, `<?php
$total = 0;
$tag = "";
for ($i = 0; $i < 100; $i++) {
	$total = ($total + $i * 3) % 97;
	$tag = $total > 48 ? "hi" : "lo";
	if ($tag === "hi") {
		$total = $total + strlen($tag . $i);
	}
}
echo $total, " ", $tag;
`)
}

// BenchmarkScriptInterp is the same loop shape over interpolated literals: the
// per-iteration work is a double-quoted string that embeds a variable, an
// array subscript and a property, which is what a template render is made of.
// The parse happens once, so the number is the per-execution cost of the
// literal.
func BenchmarkScriptInterp(b *testing.B) {
	benchmarkScript(b, `<?php
class Row {
	var $id = "r1";
}
$row = new Row();
$user = array("name" => "Ada");
$out = "";
for ($i = 0; $i < 100; $i++) {
	$out = "user=$user[name] id=$row->id n=$i";
}
echo $out;
`)
}

// BenchmarkScriptConcat is the control for BenchmarkScriptInterp: the same
// string, written as the concatenation a reader would write by hand. The two
// read against each other, because an interpolated literal is a concatenation
// and should not cost more than one.
func BenchmarkScriptConcat(b *testing.B) {
	benchmarkScript(b, `<?php
class Row {
	var $id = "r1";
}
$row = new Row();
$user = array("name" => "Ada");
$out = "";
for ($i = 0; $i < 100; $i++) {
	$out = "user=" . $user["name"] . " id=" . $row->id . " n=" . $i;
}
echo $out;
`)
}
