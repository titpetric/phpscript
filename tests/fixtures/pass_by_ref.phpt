name: foreach by value and by reference
flatstack: true
description: >
  `foreach ($a as $v)` binds a copy of the element, so writing through the loop
  variable leaves the source alone; `foreach ($a as &$v)` binds the element, so
  it does not. A write made before break, continue or return is kept, because a
  reference makes the element live. Both engines run this fixture, which is
  what pins them to one behaviour. Objects are covered by
  pass_by_ref_objects.phpt, since the bytecode engine has no class declarations.
---
<?php

// By value: the loop variable is a copy, whatever is written through it.
$scalars = array(1, 2);
foreach ($scalars as $value) {
	$value = 99;
}
echo "value scalar: " . implode(",", $scalars) . "\n";

$rows = array(array("n" => 1), array("n" => 2));
foreach ($rows as $row) {
	$row["n"] = 99;
	$row["added"] = "x";
}
echo "value array: " . $rows[0]["n"] . "," . $rows[1]["n"];
echo isset($rows[0]["added"]) ? " leaked\n" : " clean\n";

$deep = array(array("a" => array("b" => 1)));
foreach ($deep as $outer) {
	$outer["a"]["b"] = 99;
}
echo "value nested: " . $deep[0]["a"]["b"] . "\n";

// By reference: the loop variable is the element.
$doubled = array(1, 2, 3);
foreach ($doubled as &$each) {
	$each = $each * 10;
}
unset($each);
echo "ref scalar: " . implode(",", $doubled) . "\n";

$refRows = array(array("n" => 1), array("n" => 2));
foreach ($refRows as &$refRow) {
	$refRow["n"] = $refRow["n"] * 10;
}
unset($refRow);
echo "ref array: " . $refRows[0]["n"] . "," . $refRows[1]["n"] . "\n";

$keyed = array("x" => 1, "y" => 2);
foreach ($keyed as $key => &$keyedValue) {
	$keyedValue = $key . ":" . $keyedValue;
}
unset($keyedValue);
echo "ref keyed: " . $keyed["x"] . " " . $keyed["y"] . "\n";

// Leaving the body early keeps the write that was already made.
$broken = array(1, 2, 3);
foreach ($broken as &$item) {
	$item = $item * 10;
	if ($item == 20) {
		break;
	}
}
unset($item);
echo "ref break: " . implode(",", $broken) . "\n";

$skipped = array(1, 2, 3);
foreach ($skipped as &$skip) {
	$skip = $skip * 10;
	if ($skip == 20) {
		continue;
	}
	$skip = $skip + 1;
}
unset($skip);
echo "ref continue: " . implode(",", $skipped) . "\n";

function stopAtTwo(&$numbers) {
	foreach ($numbers as &$number) {
		$number = $number * 10;
		if ($number == 20) {
			return "stopped";
		}
	}
	return "finished";
}

$returned = array(1, 2, 3);
echo "ref return: " . stopAtTwo($returned) . " " . implode(",", $returned) . "\n";

// A nested loop by reference writes through both levels.
$grid = array(array(1, 2), array(3, 4));
foreach ($grid as &$gridRow) {
	foreach ($gridRow as &$cell) {
		$cell = $cell * 2;
	}
	unset($cell);
}
unset($gridRow);
echo "ref nested: " . implode(",", $grid[0]) . " " . implode(",", $grid[1]) . "\n";

// A loop that only reads leaves the source alone either way.
$read = array(1, 2);
foreach ($read as $only) {
	echo "read " . $only . " ";
}
echo "| " . implode(",", $read) . "\n";
---
value scalar: 1,2
value array: 1,2 clean
value nested: 1
ref scalar: 10,20,30
ref array: 10,20
ref keyed: x:1 y:2
ref break: 10,20,3
ref continue: 11,20,31
ref return: stopped 10,20,3
ref nested: 2,4 6,8
read 1 read 2 | 1,2
