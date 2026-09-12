name: type reassignment rules
description: >
  The writes the type-immutability rule allows and refuses, on both engines
  (a deliberate divergence from PHP; docs/README.md). Allowed: null before
  the first real value declares nothing, int widens to float because PHP
  arithmetic does (overflow, uneven division), and false is the stdlib's
  absence sentinel, writable over any type and constraining nothing when a
  real value follows. Refused, as a catchable RuntimeException: another
  type over a declared one, whether by assignment, compound assignment or
  ++ on a numeric string, and null over a declared type - a variable is
  released with unset(), not laundered through null.
runner:
  php: false
---
<?php

// null before the first real value declares nothing.
$v = null;
$v = "text";
echo "null-first: ", $v, "\n";

// int and float are one number class; PHP's own arithmetic widens.
$n = 7;
$n = $n / 2;
echo "widened: ", $n, "\n";

// false is the absence sentinel: the fgetcsv/readdir loop shape.
$row = array(1, 2);
$row = false;
$row = array(3, 4);
echo "sentinel: ", $row[0], ",", $row[1], "\n";

// Another type over a declared one throws.
try {
	$s = "text";
	$s = 1;
} catch (RuntimeException $e) {
	echo "caught: ", $e->getMessage(), "\n";
}

// The compound spelling is the same write.
try {
	$c = 5;
	$c .= "x";
} catch (RuntimeException $e) {
	echo "caught: ", $e->getMessage(), "\n";
}

// ++ on a numeric string produces an int in PHP, which is the same
// reassignment; the Perl-style carry ("a"++, "z"++) stays a string and runs.
try {
	$i = "9";
	$i++;
} catch (RuntimeException $e) {
	echo "caught: ", $e->getMessage(), "\n";
}

// null does not release a declared type; unset() does.
try {
	$u = "text";
	$u = null;
} catch (RuntimeException $e) {
	echo "caught: ", $e->getMessage(), "\n";
}
unset($u);
$u = 1;
echo "unset redeclares: ", $u, "\n";
---
null-first: text
widened: 3.5
sentinel: 3,4
caught: no reassignment: $s previously declared as string
caught: no reassignment: $c previously declared as int
caught: no reassignment: $i previously declared as string
caught: no reassignment: $u previously declared as string, null does not unset it
unset redeclares: 1
