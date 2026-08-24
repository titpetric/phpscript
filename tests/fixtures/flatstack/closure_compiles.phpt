name: closures compile to bytecode
description: >
  An anonymous function is a *model.Closure node. The flatstack compiler lays
  its body out inline and leaves an instruction that builds the callable, so a
  file that hands usort() or array_map() a comparator stays on bytecode instead
  of dropping its whole program back to the interpreter.
  tests/fixtures/functions/closure_capture.phpt covers the capture rules
  themselves; this one covers the compiled path.
---
<?php

$words = array("ccc", "a", "bb");
usort($words, function ($a, $b) {
	return strlen($a) - strlen($b);
});
echo implode(",", $words), "\n";

$factor = 3;
$scale = function ($n) use ($factor) {
	return $n * $factor;
};
$factor = 10;
echo implode(",", array_map($scale, array(1, 2, 3))), "\n";

class Tagger
{
	public $prefix = "tag";

	function tag($items)
	{
		return array_map(function ($item) {
			return $this->prefix . ":" . $item;
		}, $items);
	}
}

$tagger = new Tagger();
echo implode(",", $tagger->tag(array("a", "b"))), "\n";

echo call_user_func(static function () {
	$total = 0;
	foreach (array(1, 2, 3) as $n) {
		$total += $n;
	}
	return $total;
}), "\n";

try {
	array_map(function ($n) {
		throw new Exception("no " . $n);
	}, array(1));
	echo "not reached\n";
} catch (Exception $error) {
	echo "caught: ", $error->getMessage(), "\n";
}
---
a,bb,ccc
3,6,9
tag:a,tag:b
6
caught: no 1
