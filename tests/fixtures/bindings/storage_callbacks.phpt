name: a method on a bound Go value takes every callable spelling
runner:
  php: false
description: >
  keep() declares its callback as func(key, value string) bool and walk()
  declares the uniform func(...any) (any, error): both are filled by every
  spelling php calls a callable, and by a closure built on either engine. The
  method path reached neither shape until it was planned through coerceArgOn,
  so a script was told a Closure is not a callable. Only phpscript defines the
  expected output; Storage is a host binding and php has no such name.
---
<?php

function long_enough($key, $value) {
	return strlen($value) > 1;
}

class Filter {
	public $min = 1;

	public function keep($key, $value) {
		return strlen($value) > $this->min;
	}

	public static function always($key, $value) {
		return true;
	}

	public function __invoke($key, $value) {
		return $key === "b";
	}
}

function names($records) {
	$out = array();
	foreach ($records as $record) {
		$out[] = $record->key;
	}
	return implode(",", $out);
}

$storage = new Storage;
$storage->set("a", "1");
$storage->set("b", "22");
$storage->set("c", "333");

$filter = new Filter;

echo "closure: ", names($storage->keep(function ($key, $value) { return $key !== "a"; })), "\n";
echo "name: ", names($storage->keep("long_enough")), "\n";
echo "static: ", names($storage->keep("Filter::always")), "\n";
echo "array object: ", names($storage->keep(array($filter, "keep"))), "\n";
echo "array class: ", names($storage->keep(array("Filter", "always"))), "\n";
echo "invoke: ", names($storage->keep($filter)), "\n";

echo "walk: ";
$storage->walk(function ($key, $value) {
	echo $key, "=", $value, ";";
	return null;
});
echo "\n";

try {
	$storage->keep(function ($key, $value) { throw new Exception("no:" . $key); });
	echo "not reached\n";
} catch (Exception $error) {
	echo "threw: ", $error->getMessage(), "\n";
}

try {
	$storage->walk(function ($key, $value) { throw new Exception("stop:" . $key); });
	echo "not reached\n";
} catch (Exception $error) {
	echo "walk threw: ", $error->getMessage(), "\n";
}

try {
	$storage->keep("no_such_function");
	echo "not reached\n";
} catch (Throwable $error) {
	echo "refused: ", $error->getMessage(), "\n";
}
?>
---
closure: b,c
name: b,c
static: a,b,c
array object: b,c
array class: a,b,c
invoke: b
walk: a=1;b=22;c=333;
threw: no:a
walk threw: stop:a
refused: keep(): Argument #1 must be of type callable, string given
