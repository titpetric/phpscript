name: unset, array union and auto-vivification
description: >
  unset() removes an entry without renumbering the ones around it, `+` on two
  arrays is a union rather than addition, and writing through a missing index
  creates the arrays on the way down, each level reading its own key once.
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

// Every level the write creates runs its own key once, and a receiver that is
// a call runs once.
class Calls {
	public static $n = 0;
}

function k($name) {
	Calls::$n += 1;
	return $name;
}

$deep = array();
$deep[k("l1")][k("l2")] = "leaf";
echo $deep["l1"]["l2"] . "|" . Calls::$n . "\n";

class Bag {
	public $rows = null;
}

function hold($bag) {
	Calls::$n += 1;
	return $bag;
}

$bag = new Bag;
Calls::$n = 0;
hold($bag)->rows["k"] = "held";
echo $bag->rows["k"] . "|" . Calls::$n . "\n";
---
a,c|2
x,z|2
2
a=1 b=2 c=3 
deep
leaf
unset
leaf|2
held|1
