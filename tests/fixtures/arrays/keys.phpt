name: array keys are integers only when spelled canonically
description: >
  PHP has int keys and string keys and nothing else, and the conversion is
  exact rather than permissive: a string becomes an integer key only when it
  reads back identically from the integer it would become. The expected
  section is php's own output, so the matrix run compares against the
  reference implementation.
---
<?php

$canonical = array("1", "0", "-2", "9223372036854775807");
foreach ($canonical as $s) {
	$a = array();
	$a[$s] = 1;
	foreach ($a as $k => $v) { var_dump($k); }
}

// Not canonical spellings, so these stay string keys.
$literal = array("01", "007", "00", "-01", "-0", "+1", " 1", "1 ", "1.0", "1e3", "9223372036854775808", "");
foreach ($literal as $s) {
	$a = array();
	$a[$s] = 1;
	foreach ($a as $k => $v) { var_dump($k); }
}

// The other scalars become one of the two kinds too.
$mixed = array();
$mixed[null] = "null";
$mixed[true] = "true";
$mixed[false] = "false";
foreach ($mixed as $k => $v) { var_dump($k); }

// A float key truncates toward zero, so 1.7 and 1 are one entry.
$floats = array();
$floats[1.7] = "a";
$floats[1] = "b";
$floats[-1.7] = "c";
var_dump(count($floats));
foreach ($floats as $k => $v) { var_dump($k, $v); }

// "08" and 8 are two entries, which is the whole point of the rule.
$two = array();
$two["08"] = "string";
$two[8] = "int";
var_dump(count($two));
---
int(1)
int(0)
int(-2)
int(9223372036854775807)
string(2) "01"
string(3) "007"
string(2) "00"
string(3) "-01"
string(2) "-0"
string(2) "+1"
string(2) " 1"
string(2) "1 "
string(3) "1.0"
string(3) "1e3"
string(19) "9223372036854775808"
string(0) ""
string(0) ""
int(1)
int(0)
int(2)
int(1)
string(1) "b"
int(-1)
string(1) "c"
int(2)
