name: one value renders one way in every string context
description: >
  A value reaching a string context renders the same whether it goes through
  echo, concatenation, interpolation or a binding. There used to be two
  renderers and they disagreed: the runner rendered a float at php's precision
  of 14 and the value helpers rendered the shortest form that round-trips, so
  echo and implode printed one float two ways and array_unique, which keys on
  the string form, failed to dedupe values php treats as equal. An array is
  "Array" in every one of them, and an object has no string form at all, there
  being no __toString in this runtime, so a string context refuses it the way
  php does. The expected output is what php 8.5 prints for this source, with
  php's "Array to string conversion" warnings on stderr rather than stdout.
---
<?php

class Point
{
	public $x = 1;
}

// The same float through echo, concatenation, interpolation and a binding.
$f = 0.1 * 0.2;
echo $f, "\n";
echo "" . $f, "\n";
echo "$f\n";
echo implode(",", [$f]), "\n";
echo str_replace("x", $f, "x"), "\n";
echo sprintf("%s", $f), "\n";

$third = 1 / 3;
echo $third, "\n";
echo implode(",", [$third]), "\n";
echo sprintf("%s", $third), "\n";

// The exponent form, which is php's and not Go's.
echo implode(",", [1.0e20, 1.0e-7, 2.0, 1234.5]), "\n";
echo 1.0e20, " ", 1.0e-7, " ", 2.0, " ", 1234.5, "\n";

// array_unique keys on the string form, so two spellings of one value collapse.
var_dump(array_unique([0.1 * 0.2, 0.02]));
var_dump(in_array("0.02", [0.1 * 0.2]));

// An array is "Array" wherever a string is wanted.
echo implode(",", [[1], [2]]), "\n";
echo strlen(implode("", [[1]])), "\n";
echo sprintf("%s", [1, 2]), "\n";
echo [1, 2], "\n";
echo "x" . [1], "\n";

// An object has no string form; a string context raises php's Error.
try {
	echo implode(",", [new Point()]);
} catch (Error $e) {
	echo get_class($e), ": ", $e->getMessage(), "\n";
}
try {
	echo sprintf("%s", new Point());
} catch (Error $e) {
	echo get_class($e), ": ", $e->getMessage(), "\n";
}
?>
---
0.02
0.02
0.02
0.02
0.02
0.02
0.33333333333333
0.33333333333333
0.33333333333333
1.0E+20,1.0E-7,2,1234.5
1.0E+20 1.0E-7 2 1234.5
array(1) {
  [0]=>
  float(0.020000000000000004)
}
bool(false)
Array,Array
5
Array
Array
xArray
Error: Object of class Point could not be converted to string
Error: Object of class Point could not be converted to string
