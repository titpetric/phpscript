name: increment decrement
description: PHP ++/-- semantics per type, including the Perl-style string increment with carry, no-effect cases, and live scope reads in nested expressions.
---
<?php
$i = 5;
var_dump($i++);
var_dump($i);
var_dump(++$i);
$f = 1.5; $f++;
var_dump($f);
$n = null; $n++;
var_dump($n);
$n = null; $n--;
var_dump($n);
$b = true; $b++;
var_dump($b);
$s = "5"; $s++;
var_dump($s);
$s = "5.5"; $s++;
var_dump($s);
$s = "a"; $s++;
var_dump($s);
$s = "z"; $s++;
var_dump($s);
$s = "Az"; $s++;
var_dump($s);
$s = "Zz9"; $s++;
var_dump($s);
$s = "a-z"; $s++;
var_dump($s);
$s = "9z"; $s++;
var_dump($s);
$s = "a"; $s--;
var_dump($s);
$s = ""; $s++;
var_dump($s);
$s = ""; $s--;
var_dump($s);
$x = 3;
$y = $x++ + $x++;
var_dump($y, $x);
$a = ['k' => 1];
$a['k']++;
var_dump($a['k']);
$w = "aa";
var_dump($w++ . $w);
---
int(5)
int(6)
int(7)
float(2.5)
int(1)
NULL
bool(true)
int(6)
float(6.5)
string(1) "b"
string(2) "aa"
string(2) "Ba"
string(4) "AAa0"
string(3) "a-a"
string(3) "10a"
string(1) "a"
string(1) "1"
int(-1)
int(7)
int(5)
int(2)
string(4) "aaab"
