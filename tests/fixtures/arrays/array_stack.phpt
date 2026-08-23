name: array_shift, array_unshift, array_pop and array_push
description: >
  Shifting and unshifting renumber the integer keys from zero and leave string
  keys where they are. Popping resets the next integer key, so a later append
  takes the slot the popped element had. Shifting or popping an empty array is
  null; pushing and unshifting return the new count.
---
<?php
$b = array(5 => "a", "k" => "b", 9 => "c");
var_dump(array_shift($b));
var_dump($b);
$c = array("k" => "b", 3 => "z");
var_dump(array_unshift($c, "first"));
var_dump($c);
$a = array("x", "y", "z");
var_dump(array_pop($a));
$a[] = "w";
var_dump($a);
$d = array("x");
var_dump(array_push($d, "y", "z"));
var_dump($d);
$empty = array();
var_dump(array_pop($empty));
var_dump(array_shift($empty));
---
string(1) "a"
array(2) {
  ["k"]=>
  string(1) "b"
  [0]=>
  string(1) "c"
}
int(3)
array(3) {
  [0]=>
  string(5) "first"
  ["k"]=>
  string(1) "b"
  [1]=>
  string(1) "z"
}
string(1) "z"
array(3) {
  [0]=>
  string(1) "x"
  [1]=>
  string(1) "y"
  [2]=>
  string(1) "w"
}
int(3)
array(3) {
  [0]=>
  string(1) "x"
  [1]=>
  string(1) "y"
  [2]=>
  string(1) "z"
}
NULL
NULL
