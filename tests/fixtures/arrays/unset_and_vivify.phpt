name: unset, array union and auto-vivification
description: >
  unset() removes an entry without renumbering the ones around it, `+` on two
  arrays is a union rather than addition, and writing through a missing index
  creates the arrays on the way down.
---
<?php

$row = array("a" => 1, "b" => 2, "c" => 3);
unset($row["b"]);
echo implode(",", array_keys($row)) . "|" . count($row) . "\n";

$list = array("x", "y", "z");
unset($list[1]);
echo implode(",", $list) . "|" . count($list) . "\n";

unset($row["missing"]);
echo count($row) . "\n";

$left = array("a" => 1, "b" => 2);
$right = array("b" => 99, "c" => 3);
$union = $left + $right;
foreach ($union as $key => $value) {
	echo $key . "=" . $value . " ";
}
echo "\n";

$tree = array();
$tree["one"]["two"]["three"] = "deep";
echo $tree["one"]["two"]["three"] . "\n";

class Node {
	public $children = array();
}

$node = new Node;
$node->children["left"][] = "leaf";
echo $node->children["left"][0] . "\n";

$counter = 0;
$counter += 5;
unset($counter);
echo isset($counter) ? "set" : "unset";
echo "\n";
---
a,c|2
x,z|2
2
a=1 b=2 c=3 
deep
leaf
unset
