name: math pow sqrt log
description: >
  pow follows the same int and overflow rules as the ** operator, which the
  pow(2, 63) line asserts; sqrt and log always return a float.
---
<?php
var_dump(pow(2, 3));
var_dump(pow(2, -1));
var_dump(pow(2.0, 3));
var_dump(pow("2", "3"));
var_dump(pow(-2, 63));
var_dump(pow(2, 63));
echo pow(2, 63), "\n";
echo 2 ** 63, "\n";
var_dump(pow(2, 63) === 2 ** 63);
var_dump(sqrt(16));
var_dump(sqrt(2));
var_dump(log(M_E));
var_dump(log(8, 2));
var_dump(log(100, 10));
var_dump(M_PI);
var_dump(M_E);
---
int(8)
float(0.5)
float(8)
int(8)
int(-9223372036854775808)
float(9.223372036854776E+18)
9.2233720368548E+18
9.2233720368548E+18
bool(true)
float(4)
float(1.4142135623730951)
float(1)
float(3)
float(2)
float(3.141592653589793)
float(2.718281828459045)
