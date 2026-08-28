name: end and reset read the array edges
description: >
  reset returns the first value and end the last, false for an empty array.
  There is no internal pointer in this array model, so the value is the whole
  contract, which is also all the code in the wild reads them for.
---
<?php

$a = array("x" => 1, "y" => 2, "z" => 3);
var_dump(reset($a));
var_dump(end($a));
$empty = array();
var_dump(reset($empty));
var_dump(end($empty));
$list = array("only");
var_dump(end($list) === reset($list));
---
int(1)
int(3)
bool(false)
bool(false)
bool(true)
