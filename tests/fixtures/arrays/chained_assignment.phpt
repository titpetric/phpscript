name: chained assignment allocates one array per name
description: >
  `$a = $b = array()` binds a fresh array to each name, as in PHP, because the
  parser splits a chain that ends in an array literal into one assignment per
  name rather than letting them share the one array the chain allocates. The
  split reaches a brace-less body and a function body, and preserves PHP's
  right-to-left order, which is observable when the targets overlap. A chain
  ending in anything but a literal is not split: `$m = $n = $rows` shares one
  array, the handle semantics `phpscript lint` reports.
---
<?php
$a = $b = array();
$a[] = 1;
echo count($a), count($b), "\n";

$x = $y = $z = array("k" => "v");
$x["n"] = 1;
echo count($x), count($y), count($z), "\n";

if (true) $p = $q = array();
$p[] = 1;
echo count($p), count($q), "\n";

function split_in_body()
{
	$g = $h = array();
	$g[] = 1;
	return count($g) . count($h);
}
echo split_in_body(), "\n";

// Right to left: $r is cleared first, then the array lands under "k".
$r = array("old" => 1);
$r["k"] = $r = array();
echo count($r), "\n";

// Naming an array is not a literal, so both names keep the one array.
$rows = array(1, 2, 3);
$m = $n = $rows;
echo count($m), count($n), "\n";
---
10
211
10
10
1
33
