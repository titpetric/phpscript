name: math abs min max
description: >
  abs keeps the argument type, an int for an int and a float for a float;
  min and max compare with PHP 8 semantics and return the element itself.
---
<?php
var_dump(abs(-5));
var_dump(abs(-5.5));
var_dump(abs("-3"));
var_dump(abs("-3.5"));
var_dump(abs(5));
var_dump(min(1, "2", 3));
var_dump(max(1, "2", 3));
var_dump(min([4, 2, 8]));
var_dump(max([4, 2, 8]));
var_dump(max("apple", "banana"));
var_dump(min(-1.5, 2));
var_dump(max([1, 2], [1, 3]));
---
int(5)
float(5.5)
int(3)
float(3.5)
int(5)
int(1)
int(3)
int(2)
int(8)
string(6) "banana"
float(-1.5)
array(2) {
  [0]=>
  int(1)
  [1]=>
  int(3)
}
