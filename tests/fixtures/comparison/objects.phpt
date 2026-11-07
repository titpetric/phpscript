name: instances and arrays under the comparison operators
description: >
  PHP's comparison table for the two kinds no scalar rule covers. Two
  instances of one class compare property by property over the left's order,
  two classes that differ do not compare at all, and an instance is greater
  than an array or a string while a null takes the pair into the bool domain.
  An array against a bool or a null reads as its own emptiness, which is what
  makes `[] == false` and `[1] == true`. usort over the ordering sorts, where
  a comparator answering 0 for every pair left the order alone.
---
<?php

class Point { public $x = 1; public $y = 2; }
class Pair { public $x = 1; public $y = 2; }

$a = new Point;
$b = new Point;
$high = new Point;
$high->x = 9;
$other = new Pair;

// Two instances of one class compare property by property, over the left's own
// order. Identity is ===, which reads no property.
var_dump($a == $b, $a === $b, $a == $a, $a === $a);
var_dump($a <=> $b, $a <=> $high, $high <=> $a);
var_dump($a < $high, $high > $a, $a <= $b, $a >= $b);

// Two classes that differ do not compare, whatever their properties hold.
var_dump($a == $other, $a <=> $other, $other <=> $a);

// An instance against a value of another kind: the bool domain takes a null,
// and an instance is greater than an array or a string.
var_dump($a <=> null, null <=> $a, $a <=> true);
var_dump($a <=> [1, 2], [1, 2] <=> $a, "s" <=> $a, $a <=> "s");

// The idiom the ordering exists for.
$rows = [new Point, new Point, new Point];
$rows[0]->x = 3;
$rows[1]->x = 1;
$rows[2]->x = 2;
usort($rows, function ($l, $r) { return $l <=> $r; });
echo implode(",", array_map(function ($p) { return $p->x; }, $rows)), "\n";

// An array against a bool or a null reads as its own emptiness.
var_dump(null <=> [], false <=> [], true <=> [], true <=> [1], [] <=> false, [1] <=> true);
var_dump(0 <=> [], "" <=> [], [] <=> 0);
var_dump([] == false, [1] == true, [] == null);
?>
---
bool(true)
bool(false)
bool(true)
bool(true)
int(0)
int(-1)
int(1)
bool(true)
bool(true)
bool(true)
bool(true)
bool(false)
int(1)
int(1)
int(1)
int(-1)
int(0)
int(1)
int(-1)
int(-1)
int(1)
1,2,3
int(0)
int(0)
int(1)
int(0)
int(0)
int(0)
int(-1)
int(-1)
int(1)
bool(true)
bool(true)
bool(true)
