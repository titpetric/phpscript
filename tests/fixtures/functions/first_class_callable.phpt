name: first-class callable syntax
description: >
  PHP 8.1's `callable(...)` takes the callable a call site names without calling
  it: a free function, a registered binding, a static method, a method bound to
  its receiver, a name held in a variable, and a closure, which answers itself.
  The value is a Closure, so get_class, is_callable and a callable parameter all
  take it.
---
<?php

function greet($name) { return "hi " . $name; }

class Greeter {
	static function shout($name) { return "HI " . $name; }
	function method($name) { return "method " . $name; }
}

$obj = new Greeter();

$fn = greet(...);
var_dump($fn("a"));
var_dump(get_class($fn));

$len = strlen(...);
var_dump($len("abcd"));

$shout = Greeter::shout(...);
var_dump($shout("a"));

$method = $obj->method(...);
var_dump($method("a"));

$name = "substr";
$sub = $name(...);
var_dump($sub("abcdef", 1, 3));

$closure = function ($name) { return "closure " . $name; };
$again = $closure(...);
var_dump($again("a"));
var_dump($fn(...) === $fn);

$static = "Greeter::shout";
$fromString = $static(...);
var_dump($fromString("b"));

var_dump(call_user_func(greet(...), "c"));
var_dump(array_map(strtoupper(...), array("a", "b")));
var_dump(is_callable(greet(...)));
?>
---
string(4) "hi a"
string(7) "Closure"
int(4)
string(4) "HI a"
string(8) "method a"
string(3) "bcd"
string(9) "closure a"
bool(true)
string(4) "HI b"
string(4) "hi c"
array(2) {
  [0]=>
  string(1) "A"
  [1]=>
  string(1) "B"
}
bool(true)
