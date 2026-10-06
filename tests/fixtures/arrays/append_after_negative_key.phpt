name: the append index after a negative integer key
description: >
  Where `$a[] = v` lands once the array has held a negative integer key. PHP 8.3
  changed this: the first integer key sets the next index to that key plus one
  whatever its sign, and later keys only raise it, so `[-5 => "a"]` followed by
  an append lands on -4 where PHP 7 landed on 0. An array that never held an
  integer key appends at 0, and a string key does not count as one. unset does
  not renumber and does not lower the index, while array_pop lowers it to the
  key it removed, which is why popping the only element and then inserting a
  negative key still appends at 0. A numeric-string key is the integer it
  spells and a float key truncates toward zero before it sets the index. The
  expected output is what php 8.5 prints for this source.
---
<?php

$a = [-5 => "a"];
$a[] = "b";
var_dump(array_keys($a));

// It only ever rises, so a lower key later does not pull it back.
$b = [-5 => "a"];
$b[-100] = "x";
$b[] = "b";
var_dump(array_keys($b));

// Rising from a lower first key.
$c = [-100 => "a"];
$c[-5] = "x";
$c[] = "b";
var_dump(array_keys($c));

// A string key does not set it, so the negative integer after one still does.
$d = ["k" => "a"];
$d[-5] = "x";
$d[] = "b";
var_dump(array_keys($d));

// A numeric string key is the integer it spells.
$e = ["-5" => "a"];
$e[] = "b";
var_dump(array_keys($e));

// A float key truncates toward zero before it sets the index.
$f = [];
$f[-5.9] = "a";
$f[] = "b";
var_dump(array_keys($f));

// unset does not renumber and does not lower the index.
$g = [-5 => "a"];
unset($g[-5]);
$g[] = "b";
var_dump(array_keys($g));

$h = [-5 => "a", -4 => "b"];
unset($h[-5], $h[-4]);
$h[] = "c";
var_dump(array_keys($h));

// A literal list after a negative key continues from it.
$i = [-5 => "a", "b"];
var_dump(array_keys($i));

// Several appends walk up from the negative key.
$j = [];
$j[-10] = "a";
$j[] = "b";
$j[] = "c";
var_dump(array_keys($j));

// A positive key still behaves as it always did.
$k = [7 => "a"];
$k[] = "b";
var_dump(array_keys($k));

// An array that never held an integer key appends at zero.
$l = [];
$l[] = "a";
var_dump(array_keys($l));

$m = ["k" => "v"];
$m[] = "a";
var_dump(array_keys($m));

// array_pop lowers the index to the key it removed, so a negative key after
// that does not raise it and the next append is still zero.
$n = [1];
array_pop($n);
$n[-5] = "x";
$n[] = "b";
var_dump(array_keys($n));

// unset of the last element leaves the index past it.
$o = [1];
unset($o[0]);
$o[-5] = "x";
$o[] = "b";
var_dump(array_keys($o));

// A negative and a positive key together: the positive one wins.
$p = [-3 => "a", 2 => "b"];
$p[] = "c";
var_dump(array_keys($p));
?>
---
array(2) {
  [0]=>
  int(-5)
  [1]=>
  int(-4)
}
array(3) {
  [0]=>
  int(-5)
  [1]=>
  int(-100)
  [2]=>
  int(-4)
}
array(3) {
  [0]=>
  int(-100)
  [1]=>
  int(-5)
  [2]=>
  int(-4)
}
array(3) {
  [0]=>
  string(1) "k"
  [1]=>
  int(-5)
  [2]=>
  int(-4)
}
array(2) {
  [0]=>
  int(-5)
  [1]=>
  int(-4)
}
array(2) {
  [0]=>
  int(-5)
  [1]=>
  int(-4)
}
array(1) {
  [0]=>
  int(-4)
}
array(1) {
  [0]=>
  int(-3)
}
array(2) {
  [0]=>
  int(-5)
  [1]=>
  int(-4)
}
array(3) {
  [0]=>
  int(-10)
  [1]=>
  int(-9)
  [2]=>
  int(-8)
}
array(2) {
  [0]=>
  int(7)
  [1]=>
  int(8)
}
array(1) {
  [0]=>
  int(0)
}
array(2) {
  [0]=>
  string(1) "k"
  [1]=>
  int(0)
}
array(2) {
  [0]=>
  int(-5)
  [1]=>
  int(0)
}
array(2) {
  [0]=>
  int(-5)
  [1]=>
  int(1)
}
array(3) {
  [0]=>
  int(-3)
  [1]=>
  int(2)
  [2]=>
  int(3)
}
