name: array_slice reads its length and preserve_keys as php does
description: >
  array_slice takes four parameters and only the first two were honoured. An
  omitted or null $length ran to the end rather than yielding nothing, a negative
  $length stops that many elements short of the end, and $preserve_keys asks for
  the original integer keys. String keys are kept whatever $preserve_keys says,
  which is php's rule and the reason the result is not always a list.

  The fourth argument used to be refused outright: it landed in a variadic typed
  int64 and failed coercion with "Argument #4 must be of type int, bool given".
---
<?php

$list = array(10, 20, 30, 40, 50);
$assoc = array("x" => 1, "y" => 2, "z" => 3, "w" => 4);
$mixed = array("a" => 1, 7 => 2, "b" => 3, 9 => 4);

echo "-- length omitted, negative, null --\n";
var_dump(array_slice($list, 1));
var_dump(array_slice($list, 1, -1));
var_dump(array_slice($list, 1, null));
var_dump(array_slice($list, 0, -10));

echo "-- negative offset --\n";
var_dump(array_slice($list, -2));
var_dump(array_slice($list, -2, 1));

echo "-- preserve_keys over a list --\n";
var_dump(array_slice($list, 1, 2));
var_dump(array_slice($list, 1, 2, true));

echo "-- string keys are kept either way --\n";
var_dump(array_slice($assoc, 1, 2));
var_dump(array_slice($assoc, 1, 2, true));

echo "-- mixed keys --\n";
var_dump(array_slice($mixed, 1, 3));
var_dump(array_slice($mixed, 1, 3, true));
---
-- length omitted, negative, null --
array(4) {
  [0]=>
  int(20)
  [1]=>
  int(30)
  [2]=>
  int(40)
  [3]=>
  int(50)
}
array(3) {
  [0]=>
  int(20)
  [1]=>
  int(30)
  [2]=>
  int(40)
}
array(4) {
  [0]=>
  int(20)
  [1]=>
  int(30)
  [2]=>
  int(40)
  [3]=>
  int(50)
}
array(0) {
}
-- negative offset --
array(2) {
  [0]=>
  int(40)
  [1]=>
  int(50)
}
array(1) {
  [0]=>
  int(40)
}
-- preserve_keys over a list --
array(2) {
  [0]=>
  int(20)
  [1]=>
  int(30)
}
array(2) {
  [1]=>
  int(20)
  [2]=>
  int(30)
}
-- string keys are kept either way --
array(2) {
  ["y"]=>
  int(2)
  ["z"]=>
  int(3)
}
array(2) {
  ["y"]=>
  int(2)
  ["z"]=>
  int(3)
}
-- mixed keys --
array(3) {
  [0]=>
  int(2)
  ["b"]=>
  int(3)
  [1]=>
  int(4)
}
array(3) {
  [7]=>
  int(2)
  ["b"]=>
  int(3)
  [9]=>
  int(4)
}
