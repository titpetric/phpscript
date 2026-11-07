name: spaceship operator
description: >
  Covers the spaceship operator: ordering of integers, floats, strings and
  numeric strings, arrays by count and then by element, null and bool in the
  bool domain, NAN answering 1 from either side, operands that run once in
  order, its precedence against arithmetic, and the usort idiom.
---
<?php

// Integers, strings, floats, and numeric strings.
echo 2 <=> 1, 1 <=> 2, 1 <=> 1, "\n";
echo "a" <=> "b", "b" <=> "a", "a" <=> "a", "\n";
echo "10" <=> "9", "10" <=> "9a", "\n";
echo 1.5 <=> 1.5, 1 <=> 1.5, "\n";

// Arrays compare by count first, then element by element.
var_dump([1, 2] <=> [1, 3], [1, 2, 3] <=> [1, 2], [1, 2] <=> [1, 2]);

// Null and bool compare in the bool domain.
var_dump(null <=> false, true <=> false, null <=> 0);

// NAN is unordered and answers 1 from either side.
var_dump(NAN <=> 1, 1 <=> NAN);

// Both operands run once, left to right.
function n($v) { echo "n$v "; return $v; }
var_dump(n(1) <=> n(2));

// Precedence: arithmetic binds tighter than `<=>`.
var_dump(1 + 1 <=> 2, 1 <=> 2 + 3);

// The idiom it exists for.
$rows = [[3, 'c'], [1, 'a'], [2, 'b']];
usort($rows, function ($x, $y) { return $x[0] <=> $y[0]; });
echo implode(",", array_column($rows, 1)), "\n";
usort($rows, function ($x, $y) { return [$y[1]] <=> [$x[1]]; });
echo implode(",", array_column($rows, 1)), "\n";
---
1-10
-110
1-1
0-1
int(-1)
int(1)
int(0)
int(0)
int(1)
int(0)
int(1)
int(1)
n1 n2 int(-1)
int(0)
int(-1)
a,b,c
c,b,a
