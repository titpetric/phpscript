name: int64 precision
description: >
  PHP_INT_MAX, PHP_INT_MIN and PHP_INT_SIZE are Go's int64 bounds, and an
  integer keeps every one of its bits through the paths that would otherwise
  round it through a float64, which carries 53 bits of mantissa: number_format,
  a cast from a string, and the bitwise operators all read the integer itself.
  Overflow promotes to float, as it does in PHP.
---
<?php

// PHP_INT_MAX, PHP_INT_MIN and PHP_INT_SIZE are Go's int64 bounds.
var_dump(PHP_INT_MAX);
var_dump(PHP_INT_MIN);
var_dump(PHP_INT_SIZE);

// An integer keeps every bit through the paths that would otherwise round it
// through a float64, which carries only 53 bits of mantissa.
var_dump(PHP_INT_MAX - 1);
var_dump(PHP_INT_MIN + 1);
var_dump(number_format(PHP_INT_MAX));
var_dump(number_format(PHP_INT_MIN));
var_dump(number_format(9007199254740993));
var_dump((int)"9223372036854775807");
var_dump(intdiv(PHP_INT_MAX, 1));
var_dump(max(PHP_INT_MAX, 1));
var_dump(min(PHP_INT_MIN, 1));
var_dump(array_sum(array(PHP_INT_MAX, 0)));
var_dump(PHP_INT_MAX % 1000);

// The bitwise operators are int64 operations, so every bit survives.
var_dump(PHP_INT_MAX & PHP_INT_MAX);
var_dump(~PHP_INT_MIN);
var_dump(1 << 62);
var_dump(PHP_INT_MIN >> 1);

// Overflow promotes to float, as in PHP, and the promoted value is the one a
// float64 can carry rather than the exact sum.
var_dump(PHP_INT_MAX + 1);
var_dump(PHP_INT_MIN - 1);
---
int(9223372036854775807)
int(-9223372036854775808)
int(8)
int(9223372036854775806)
int(-9223372036854775807)
string(25) "9,223,372,036,854,775,807"
string(26) "-9,223,372,036,854,775,808"
string(21) "9,007,199,254,740,993"
int(9223372036854775807)
int(9223372036854775807)
int(9223372036854775807)
int(-9223372036854775808)
int(9223372036854775807)
int(807)
int(9223372036854775807)
int(9223372036854775807)
int(4611686018427387904)
int(-4611686018427387904)
float(9.223372036854776E+18)
float(-9.223372036854776E+18)
