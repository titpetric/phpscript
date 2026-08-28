name: variadic parameters collect the rest
description: >
  A trailing ...$rest collects every remaining argument into one array, an
  empty one when the caller stops short of it, for functions and for methods
  alike.
---
<?php

function tail($first, ...$rest)
{
	return $first . ":" . implode(",", $rest);
}
echo tail("a", "b", "c"), "\n";
echo tail("only"), "\n";

function collect(...$all)
{
	return count($all);
}
var_dump(collect());
var_dump(collect(1, 2, 3));

class Joiner
{
	public static function join($sep, ...$parts)
	{
		return implode($sep, $parts);
	}
}
echo Joiner::join("-", "x", "y", "z"), "\n";
---
a:b,c
only:
int(0)
int(3)
x-y-z
