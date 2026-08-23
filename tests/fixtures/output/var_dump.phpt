name: var_dump
description: >
  var_dump annotates every value with its type, and a string with its length in
  bytes rather than characters. Its float precision is not echo's: 0.1+0.2
  prints every digit that round-trips.
---
<?php
var_dump(array(1, "a" => true));
var_dump(1.0, "he\xcc\x81", null, false);
var_dump(array("n" => array(1)));
var_dump(0.1 + 0.2);
---
array(2) {
  [0]=>
  int(1)
  ["a"]=>
  bool(true)
}
float(1)
string(4) "hé"
NULL
bool(false)
array(1) {
  ["n"]=>
  array(1) {
    [0]=>
    int(1)
  }
}
float(0.30000000000000004)
