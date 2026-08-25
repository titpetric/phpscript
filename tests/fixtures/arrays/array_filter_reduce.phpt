name: array_filter_reduce
description: >
  array_filter keeps the elements its callback accepts and preserves their
  keys, with ARRAY_FILTER_USE_KEY and ARRAY_FILTER_USE_BOTH choosing what the
  callback is given; array_reduce folds the values left to right.
---
<?php
// Without a callback the values are judged on truthiness, and the surviving
// keys keep the numbers they had.
$values = array(0, 1, "", null, "0", "a", false, 2);
echo json_encode(array_filter($values)) . "\n";

$scores = array("ann" => 90, "bob" => 40, "cid" => 75);
echo json_encode(array_filter($scores, function ($score) {
	return $score >= 70;
})) . "\n";

// ARRAY_FILTER_USE_KEY passes the key alone.
echo json_encode(array_filter($scores, function ($name) {
	return $name != "bob";
}, ARRAY_FILTER_USE_KEY)) . "\n";

// ARRAY_FILTER_USE_BOTH passes the value first and the key second.
echo json_encode(array_filter($scores, function ($score, $name) {
	return $score >= 70 && $name != "cid";
}, ARRAY_FILTER_USE_BOTH)) . "\n";

echo ARRAY_FILTER_USE_KEY . "," . ARRAY_FILTER_USE_BOTH . "\n";

// array_reduce folds left to right, and $initial defaults to null.
$numbers = array(1, 2, 3, 4);
var_dump(array_reduce($numbers, function ($carry, $item) {
	return $carry + $item;
}, 0));
var_dump(array_reduce($numbers, function ($carry, $item) {
	return $carry * $item;
}, 1));
var_dump(array_reduce(array(), function ($carry, $item) {
	return $carry;
}));
echo array_reduce(array("a", "b", "c"), function ($carry, $item) {
	return $carry . $item;
}, "") . "\n";
---
{"1":1,"5":"a","7":2}
{"ann":90,"cid":75}
{"ann":90,"cid":75}
{"ann":90}
2,1
int(10)
int(24)
NULL
abc
