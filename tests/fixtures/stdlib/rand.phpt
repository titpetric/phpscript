name: rand picks from an inclusive range
description: >
  rand returns an integer between $min and $max inclusive, and between 0 and
  2147483647 without arguments. The value is random, so equal bounds and
  range membership are what an expected section can pin down.
---
<?php

var_dump(rand(5, 5));
var_dump(rand(-3, -3));
$n = rand(1, 10);
var_dump($n >= 1 && $n <= 10);
$m = rand();
var_dump($m >= 0 && $m <= 2147483647);
var_dump(rand(0, 1) === rand(0, 1) || true);
---
int(5)
int(-3)
bool(true)
bool(true)
bool(true)
