name: a method read without parentheses is a callable bound to its receiver
runner:
  php: false
description: >
  $obj->name with no parentheses answers the property of that name; where the
  class declares a method instead and no property holds one, it answers that
  method bound to $obj. The value is callable everywhere a callable is taken -
  a direct call, call_user_func, array_map - and the body sees the $this it was
  read from, so a method registered this way is a function of its arguments
  plus one shared receiver. It is what php's first-class callable syntax
  $obj->name(...) answers, which is why get_class() says Closure; php itself
  reads the shorter spelling as an undefined property and warns, so the php
  runner is opted out. HTTP\Mux takes one as a handler, which docs/README.md
  records and testdata/testserver.php is written as.
---
<?php

class Calc {
	public $factor = 3;

	public function times($n) { return $n * $this->factor; }

	// A method value is how a method hands one of its own methods to something
	// that calls it back.
	public function apply($values) { return array_map($this->times, $values); }
}

$calc = new Calc;

$times = $calc->times;
echo $times(5), "\n";
echo call_user_func($calc->times, 6), "\n";
echo call_user_func_array($calc->times, array(7)), "\n";
echo implode(",", $calc->apply(array(1, 2, 3))), "\n";
var_dump(is_callable($calc->times));
var_dump($calc->times instanceof Closure);
echo get_class($calc->times), "\n";

// A name that is neither a property nor a method is null, as reading an
// undefined property is.
var_dump($calc->missing);
?>
---
15
18
21
3,6,9
bool(true)
bool(true)
Closure
NULL
