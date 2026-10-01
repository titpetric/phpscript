name: func_get_args reads the arguments of the call in flight
description: >
  Every argument the caller passed, including the ones past the last declared
  parameter, in a function, a closure, a closure with a capture, and a method.
  The bytecode engine does not compile a program that calls it - a compiled
  frame seeds its parameters into slots and keeps no argument list - so this
  fixture reports SKIP for that column and the program delegates to the
  interpreter when a host runs it. php defines the expected output.
---
<?php

function named($first)
{
	return count(func_get_args()) . ":" . implode(",", func_get_args()) . ":" . $first;
}
echo "function: ", named("a", "b", "c"), "\n";
echo "exactly: ", named("a"), "\n";

$closure = function ($first) {
	return count(func_get_args()) . ":" . implode(",", func_get_args());
};
echo "closure: ", $closure("a", "b", "c"), "\n";

$captured = "kept";
$withUse = function ($first) use ($captured) {
	return $captured . ":" . implode(",", func_get_args());
};
echo "capture: ", $withUse("a", "b"), "\n";

class Holder
{
	public function method($first)
	{
		return implode(",", func_get_args());
	}
}
echo "method: ", (new Holder)->method("a", "b", "c"), "\n";
?>
---
function: 3:a,b,c:a
exactly: 1:a:a
closure: 3:a,b,c
capture: kept:a,b
method: a,b,c
