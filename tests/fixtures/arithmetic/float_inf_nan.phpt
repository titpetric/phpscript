name: INF and NAN are constants a script can write
description: >
  Every renderer here already spelled the two non-finite floats - var_dump,
  var_export, print_r and sprintf each name INF and NAN - but neither constant
  was registered, so a script could read one out of an overflow and never write
  one. Reaching NaN meant sqrt(-1), which is how int_conversion.phpt had to
  spell it.

  NAN compares equal to nothing, itself included, so the three comparisons are
  what tells a reader it is not a number rather than a large one. INF is
  ordered, above every finite float, and absorbs addition.

  `echo NAN` is left out: php 8.5 warns on a NaN coerced to string, and this
  runtime reports 8.4 and does not.
---
<?php
var_dump(INF, -INF, NAN);
var_dump(is_float(INF), is_float(NAN));
echo INF, " ", -INF, "\n";
var_dump(INF > PHP_FLOAT_MAX, -INF < -PHP_FLOAT_MAX);
var_dump(NAN == NAN, NAN === NAN, NAN != NAN);
var_dump(1 / INF, INF + 1, INF - INF);
var_export([INF, NAN]);
echo "\n";
---
float(INF)
float(-INF)
float(NAN)
bool(true)
bool(true)
INF -INF
bool(true)
bool(true)
bool(false)
bool(false)
bool(true)
float(0)
float(INF)
float(NAN)
array (
  0 => INF,
  1 => NAN,
)
