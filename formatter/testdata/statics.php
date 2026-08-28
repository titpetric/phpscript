<?php

function counter() {
	static $n = 0;
	static $a = 1, $b;
	global $x;
	global $first, $second;
	$n++;
	return $n;
}

class Greeter {
	public static function hello($name) {
		static $count = 0;
		$count++;
		return "hello " . $name;
	}
}

$m = 'hello';
echo Greeter::$m('world');
echo Greeter::$m("again"), "\n";
