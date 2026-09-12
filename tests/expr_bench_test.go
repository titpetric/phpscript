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
