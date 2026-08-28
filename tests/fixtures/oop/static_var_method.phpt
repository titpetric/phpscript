name: method statics are shared per method, not per instance
description: >
  A `static $x` inside a method belongs to the method, so two instances
  increment the same counter, and a static method keeps its own tally the
  same way. Verified against php 8.5.
---
<?php

class Greeter {
	public function hello() {
		static $count = 0;
		$count++;
		return "hello " . $count;
	}

	public static function visits() {
		static $n = 0;
		$n++;
		return $n;
	}
}

$a = new Greeter();
$b = new Greeter();
echo $a->hello(), "\n";
echo $b->hello(), "\n";
echo Greeter::visits(), Greeter::visits(), "\n";
---
hello 1
hello 2
12
