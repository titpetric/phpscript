name: parameter defaults bind when the caller stops short
description: >
  A parameter default is evaluated at call time when the caller omits the
  argument, for functions and methods alike. An explicit null is an argument,
  not an omission, so it does not trigger the default.
---
<?php

function greet($name, $greeting = "Hello", $punct = "!")
{
	return $greeting . ", " . $name . $punct;
}
echo greet("world"), "\n";
echo greet("world", "Hi"), "\n";
echo greet("world", "Hi", "?"), "\n";
echo greet("world", null), "\n";

class Counter
{
	public $step;
	public function __construct($step = 10)
	{
		$this->step = $step;
	}
	public function bump($by = 2)
	{
		return $this->step + $by;
	}
}
$c = new Counter();
echo $c->bump(), "\n";
echo $c->bump(5), "\n";
$d = new Counter(1);
echo $d->bump(), "\n";
---
Hello, world!
Hi, world!
Hi, world?
, world!
12
15
3
