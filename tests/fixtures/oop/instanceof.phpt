name: instanceof
description: >
  instanceof is class-name equality: it tests the class of a value against a
  bare class name, a variable holding one, or an object to take the class of. A
  scalar, null and an array are an instance of nothing. It binds tighter than
  `!`, so `!$x instanceof C` negates the test.
---
<?php

class Point {
	var $x = 1;
}

class Circle {
	var $r = 2;
}

$point = new Point();
$circle = new Circle();
$name = "Point";

var_dump($point instanceof Point);
var_dump($point instanceof Circle);
var_dump($point instanceof $name);
var_dump($point instanceof $circle);
var_dump($point instanceof $point);

var_dump(1 instanceof Point);
var_dump("Point" instanceof Point);
var_dump(null instanceof Point);
var_dump(array() instanceof Point);

var_dump(!$point instanceof Circle);
var_dump($point instanceof Point ? "yes" : "no");
---
bool(true)
bool(false)
bool(true)
bool(false)
bool(true)
bool(false)
bool(false)
bool(false)
bool(false)
bool(true)
string(3) "yes"
