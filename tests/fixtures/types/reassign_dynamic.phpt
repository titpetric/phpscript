name: variable types are dynamic
description: >
  Reassigning a variable at another type follows PHP: the variable retypes
  silently, on both engines. Runtime enforcement of one-type-per-name was
  shipped and reversed; the convention survives as an advisory phpscript
  lint finding ("no reassignment: $a previously declared as string"), which
  flags the drift without failing the run. This fixture pins the runtime
  half: every shape the enforcement used to refuse runs, and the php column
  is the oracle again.
---
<?php

$a = "text";
$a = 1;
echo $a, "\n";

$n = 5;
$n .= "x";
echo $n, "\n";

$i = "9";
$i++;
var_dump($i);

$u = "text";
$u = null;
var_dump($u);

$row = array(1, 2);
$row = false;
$row = array(3, 4);
echo $row[0], ",", $row[1], "\n";
---
1
5x
int(10)
NULL
3,4
