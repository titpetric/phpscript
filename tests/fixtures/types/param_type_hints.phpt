name: parameter and return type hints are not checked
runner:
  php: false
description: >
  PHP checks a declared parameter type on every call and a declared return type
  on every return, and raises an uncatchable-by-type TypeError when either
  disagrees: f(): Argument #1 ($n) must be of type int, string given. phpscript
  parses both and checks neither, so the argument binds at whatever type it
  arrived as and the function answers it back. var_dump therefore reports the
  caller's types rather than the declared ones, and the int-declared function
  returns a string. Only phpscript defines the expected output; php fatals on
  the first call.
---
<?php

function takes(int $n, string $s) {
	var_dump($n);
	var_dump($s);
}

takes("not an int", 42);

// A declared return type is not checked either, so the string comes back out
// of a function that promised an int.
function answers(): int {
	return "twelve";
}

var_dump(answers());

// A class type hint names a class that need not exist, because nothing looks
// it up: the argument is bound whatever it is.
function typed(\No\Such\Klass $value) {
	return $value;
}

var_dump(typed("still a string"));
---
string(10) "not an int"
int(42)
string(6) "twelve"
string(14) "still a string"
