name: math rounding
description: >
  round is half away from zero on the decimal the value prints as, so
  round(1.005, 2) is 1.01; floor and ceil always return a float.
---
<?php
var_dump(round(3.0));
var_dump(round(5));
var_dump(round(2.5));
var_dump(round(-2.5));
var_dump(round(1.005, 2));
var_dump(round(0.285, 2));
var_dump(round(1.45, 1));
var_dump(round(1234.5678, 2));
var_dump(round(1234.5678, -2));
var_dump(round(60000, -5));
var_dump(round(77.777777777777, 2));
var_dump(floor(4.7));
var_dump(ceil(4.3));
var_dump(floor(-4.7));
var_dump(ceil(-4.3));
var_dump(floor(4));
---
float(3)
float(5)
float(3)
float(-3)
float(1.01)
float(0.29)
float(1.5)
float(1234.57)
float(1200)
float(100000)
float(77.78)
float(4)
float(5)
float(-5)
float(-4)
float(4)
