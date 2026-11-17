name: go runtime pointers
runtime: go
description: >
  `&$a` takes the address of a variable and `*$p` reads and writes what it
  points at: an assignment through the pointer changes the variable, where an
  assignment to a copy does not. An element takes an address too. php reads
  `&` as assignment by reference and has no prefix `*`, so only phpscript
  defines this output.
---
<?php

$a = 1;
$p = &$a;
*$p = 2;
echo $a, "|", *$p, "\n";

// A copy is a copy: writing $b leaves $a alone.
$b = $a;
$b = 5;
echo $a, "|", $b, "\n";

// An element takes an address, and a compound assignment writes through it.
$list = [1, 2];
$q = &$list[1];
*$q += 1;
echo $list[1], "\n";

// A property takes one too, as go's `&s.f` does.
class Counter
{
	public $hits = 1;
}

$c = new Counter;
$h = &$c->hits;
*$h += 1;
echo $c->hits, "\n";
---
2|2
2|5
3
2
