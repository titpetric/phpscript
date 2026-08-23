name: gettype
description: >
  gettype reports PHP's legacy type names: integer and double rather than int
  and float, boolean rather than bool, and NULL in capitals. A constructed
  object and a closure are both objects.
---
<?php
class Thing {}
foreach (array(1, 1.5, "s", true, false, array(), null) as $value) {
	echo gettype($value), "\n";
}
echo gettype(new Thing), "\n";
echo gettype(function () { return 1; }), "\n";
---
integer
double
string
boolean
boolean
array
NULL
object
object
