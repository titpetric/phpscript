name: php callable spellings
description: >
  call_user_func and friends accept every spelling PHP calls a callable: a
  closure, a function name, "Class::method", array($object, "method") and
  array("Class", "method").
---
<?php

class Greeter {
	public $greeting = "hello";

	function hello($who) {
		return $this->greeting . " " . $who;
	}

	static function shout($who) {
		return "HELLO " . $who;
	}
}

function plain($who) {
	return "plain " . $who;
}

$greeter = new Greeter();

echo call_user_func(function ($who) { return "closure " . $who; }, "a") . "\n";
echo call_user_func("plain", "b") . "\n";
echo call_user_func(array($greeter, "hello"), "c") . "\n";
echo call_user_func_array(array($greeter, "hello"), array("d")) . "\n";
echo call_user_func("Greeter::shout", "e") . "\n";
echo call_user_func_array(array("Greeter", "shout"), array("f")) . "\n";

echo is_callable("plain") ? "callable\n" : "not callable\n";
echo is_callable("no_such_function") ? "callable\n" : "not callable\n";
echo function_exists("plain") ? "exists\n" : "missing\n";
echo function_exists("no_such_function") ? "exists\n" : "missing\n";
?>
---
closure a
plain b
hello c
hello d
HELLO e
HELLO f
callable
not callable
exists
missing
