name: closure captures and value invocation
description: >
  A closure's `use (...)` list snapshots the enclosing frame at the point the
  closure value is created, and a callable held in a value is invoked directly:
  $fn(...), $array[0](...), and a closure read back out of a property.
---
<?php

declare(strict_types=1);

function make_greeter($greeting) {
	return function ($name) use ($greeting) {
		return $greeting . ", " . $name . "!";
	};
}

$hello = make_greeter("Hello");
$hi = make_greeter("Hi");

echo $hello("world") . "\n";
echo $hi("there") . "\n";

$prefix = "before";
$show = function () use ($prefix) {
	return $prefix;
};
$prefix = "after";
echo $show() . "\n";

$table = array("upper" => function ($s) { return strtoupper($s); });
echo $table["upper"]("shout") . "\n";

$bound = Closure::bind(static function ($n) { return $n * 2; }, null, null);
echo $bound(21) . "\n";

class Box {
	public $run;

	function __construct() {
		$this->run = function ($v) { return "boxed:" . $v; };
	}
}

$box = new Box;
$run = $box->run;
echo $run("x") . "\n";
---
Hello, world!
Hi, there!
before
SHOUT
42
boxed:x
