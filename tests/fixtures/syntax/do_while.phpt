name: do-while runs the body before the condition
description: >
  `do { ... } while (cond);` executes the body at least once, even when the
  condition is false on entry; continue jumps to the condition check and
  break leaves the loop, as in the other loops. Verified against php 8.5.
---
<?php

$n = 5;
do {
	echo "runs once even when false: ", $n, "\n";
} while ($n < 5);

$i = 0;
do {
	$i++;
	if ($i == 2) {
		continue;
	}
	if ($i > 4) {
		break;
	}
	echo "i = ", $i, "\n";
} while ($i < 10);
echo "final i = ", $i, "\n";

$parts = array();
$value = 1234;
do {
	$parts[] = $value % 10;
	$value = intdiv($value, 10);
} while ($value > 0);
echo implode(",", $parts), "\n";
---
runs once even when false: 5
i = 1
i = 3
i = 4
final i = 5
4,3,2,1
