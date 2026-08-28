name: each closure value gets its own statics
description: >
  PHP gives every closure instance its own static table: two counters built
  by the same factory count independently, and each keeps its own tally
  across calls. Verified against php 8.5.
---
<?php

function make_counter() {
	return function () {
		static $n = 0;
		$n++;
		return $n;
	};
}

$a = make_counter();
$b = make_counter();
echo $a(), $a(), $a(), "\n";
echo $b(), "\n";
echo $a(), "\n";
---
123
1
4
