name: php callable spellings
description: >
  Every spelling php calls a callable, through call_user_func, call_user_func_array
  and array_map, and what is_callable answers for each: a closure, a function
  name, "Class::method", array($object, "method"), array("Class", "method") and
  an object declaring __invoke. The four refused shapes are here too, because
  "not a valid callback" is as much of the contract as the six that resolve.
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

	function __invoke($who) {
		return "invoked " . $who;
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
echo call_user_func($greeter, "g") . "\n";

echo is_callable("plain") ? "callable\n" : "not callable\n";
echo is_callable("no_such_function") ? "callable\n" : "not callable\n";
echo function_exists("plain") ? "exists\n" : "missing\n";
echo function_exists("no_such_function") ? "exists\n" : "missing\n";

$spellings = array(
	"closure" => function ($who) { return "closure " . $who; },
	"name" => "plain",
	"static string" => "Greeter::shout",
	"array object" => array($greeter, "hello"),
	"array class" => array("Greeter", "shout"),
	"invokable" => $greeter,
);
$refused = array(
	"missing name" => "no_such_function",
	"missing method" => array($greeter, "nope"),
	"int" => 42,
	"one-element array" => array($greeter),
);

foreach ($spellings as $label => $spelling) {
	echo $label, ": ", var_export(is_callable($spelling), true),
		" ", implode(",", array_map($spelling, array("x", "y"))), "\n";
}
foreach ($refused as $label => $spelling) {
	echo $label, ": ", var_export(is_callable($spelling), true), "\n";
}
?>
---
closure a
plain b
hello c
hello d
HELLO e
HELLO f
invoked g
callable
not callable
exists
missing
closure: true closure x,closure y
name: true plain x,plain y
static string: true HELLO x,HELLO y
array object: true hello x,hello y
array class: true HELLO x,HELLO y
invokable: true invoked x,invoked y
missing name: false
missing method: false
int: false
one-element array: false
