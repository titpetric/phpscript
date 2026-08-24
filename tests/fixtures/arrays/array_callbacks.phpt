name: array callbacks with closures
description: >
  usort and array_map take a closure, inline and held in a variable. The
  comparator sorts by length and then alphabetically, the shape a template
  engine uses to replace the longest variable names first. This passes on
  flatstack today by falling back to the compatibility interpreter;
  TestFlatstackSupportsArrayCallbackClosures is what holds the bytecode
  engine to compiling it.
---
<?php
$vars = array('$var', '$v', '$variable', '$var1');
usort($vars, function ($a, $b) {
	if (strlen($a) == strlen($b)) {
		if ($a == $b) {
			return 0;
		}
		return ($a < $b) ? 1 : -1;
	}
	return (strlen($a) < strlen($b)) ? 1 : -1;
});
echo implode(",", $vars), "\n";

echo implode(",", array_map(function ($n) { return $n * 2; }, array(1, 2, 3))), "\n";

$byLength = function ($a, $b) { return strlen($a) - strlen($b); };
$words = array("ccc", "a", "bb");
usort($words, $byLength);
echo implode(",", $words), "\n";
---
$variable,$var1,$var,$v
2,4,6
a,bb,ccc
