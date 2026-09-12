name: unary plus
description: Unary plus is the numeric cast 0 + $x, on literals, numeric strings, leading-numeric strings, booleans and null.
---
<?php
var_dump(+"5");
var_dump(+"5x");
var_dump(+true);
var_dump(+null);
var_dump(+1.5);
var_dump(+"1e2");
$x = "7";
var_dump(+$x);
---
int(5)
int(5)
int(1)
int(0)
float(1.5)
float(100)
int(7)
