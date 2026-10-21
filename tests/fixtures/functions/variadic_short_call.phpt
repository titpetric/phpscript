name: a call that stops short of a variadic parameter binds an empty array
description: >
  `function tail($first, $second, ...$rest)` called as `tail("x")` leaves the
  variadic parameter's index past the end of the arguments. Slicing from there
  crashed the interpreter with `slice bounds out of range [2:1]`, reaching the
  caller as a host panic instead of running the function.

  php refuses this call outright, with `ArgumentCountError: Too few arguments
  to function tail(), 1 passed and exactly 2 expected`, so the php runner is
  opted out because that is the divergence being stated. phpscript binds nil
  for a parameter the caller skipped, for a function with no variadic
  parameter as much as for this one, and the empty array is what that rule
  answers for the tail. The divergence is the padding, and no longer a crash.
runner:
  php: false
---
<?php

function tail($first, $second, ...$rest) {
	echo "first=" . var_export($first, true) . "\n";
	echo "second=" . var_export($second, true) . "\n";
	echo "rest=" . count($rest) . "\n";
}

// Short of the variadic parameter: one argument for two named ones.
tail("x");

// Exactly at it, which is the case that never crashed.
tail("x", "y");

// Past it, which collects the tail.
tail("x", "y", "z", "w");
?>
---
first='x'
second=NULL
rest=0
first='x'
second='y'
rest=0
first='x'
second='y'
rest=2
