name: property increment and decrement
description: >
  ++ and -- on an object property, in every spelling: postfix and prefix,
  reading the expression value, and mixed with compound assignment on the same
  property. Variables and array indexes already lower to flat bytecode;
  a property target selects the interpreter fallback, so this fixture pins the
  behavior both engines must share. The shape is a page counter the way
  dbadmin tallies rows and columns per table.
---
<?php

class Stats {
	public $rows = 0;
	public $columns = 10;
}

$stats = new Stats;

$stats->rows++;
$stats->rows++;
echo $stats->rows, "\n";

echo $stats->rows++, ":", $stats->rows, "\n";
echo ++$stats->rows, ":", $stats->rows, "\n";

$stats->columns--;
echo $stats->columns--, ":", $stats->columns, "\n";
echo --$stats->columns, ":", $stats->columns, "\n";

$stats->rows += 5;
$stats->rows++;
echo $stats->rows, "\n";
---
2
2:3
4:4
9:8
7:7
10
