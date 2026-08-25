name: array_reverse_sum
description: >
  array_reverse renumbers integer keys from zero unless $preserve_keys is
  set and keeps string keys either way; array_sum returns an int while every
  value reads as an integer and a float from the first float onwards.
---
<?php
function dump_array($label, $array) {
	$parts = array();
	foreach ($array as $key => $value) {
		$parts[] = $key . ":" . $value;
	}
	echo $label . " " . implode(", ", $parts) . "\n";
}

$list = array(1, 2, 3);
dump_array("list", array_reverse($list));
dump_array("list preserved", array_reverse($list, true));

// Sparse integer keys are renumbered from zero without the flag.
$sparse = array(5 => "a", 9 => "b");
dump_array("sparse", array_reverse($sparse));
dump_array("sparse preserved", array_reverse($sparse, true));

// String keys survive either way; only the integer ones are renumbered.
$mixed = array("a" => 1, 5 => 2, "b" => 3);
dump_array("mixed", array_reverse($mixed));
dump_array("mixed preserved", array_reverse($mixed, true));

$assoc = array("first" => 1, "second" => 2);
dump_array("assoc", array_reverse($assoc));
dump_array("empty", array_reverse(array()));

// array_sum keeps PHP's return type: int while every value reads as an int,
// float from the first float onwards.
var_dump(array_sum(array(1, 2, 3)));
var_dump(array_sum(array()));
var_dump(array_sum(array(1, 2.5, 3)));
var_dump(array_sum(array("1", "2.5", 3)));
var_dump(array_sum(array("10", "20")));
var_dump(array_sum(array(true, false, null, 2)));
var_dump(array_sum(array("id" => 4, "qty" => 6)));
var_dump(array_sum(array(PHP_INT_MAX, 1)));
---
list 0:3, 1:2, 2:1
list preserved 2:3, 1:2, 0:1
sparse 0:b, 1:a
sparse preserved 9:b, 5:a
mixed b:3, 0:2, a:1
mixed preserved b:3, 5:2, a:1
assoc second:2, first:1
empty 
int(6)
int(0)
float(6.5)
float(6.5)
int(30)
int(3)
int(10)
float(9.223372036854776E+18)
