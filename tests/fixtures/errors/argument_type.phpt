name: an argument of the wrong type is thrown and catchable
description: >
  An argument that converts to no declared parameter type is refused as a
  throwable a script can catch, rather than panicking in the host, so execution
  continues after the catch block. It is a TypeError, which means catch
  (TypeError) matches it and catch (Exception) does not. The two messages differ
  in detail, since PHP also names the parameter ("($limit)") and a Go binding
  cannot report that, so only the function, the argument position and the
  expected type are compared.
---
<?php
try {
	explode(",", "a,b", array());
	echo "not reached\n";
} catch (TypeError $e) {
	echo "caught ", get_class($e), "\n";
	echo strpos($e->getMessage(), "explode(): Argument #3") !== false ? "names the argument\n" : "unnamed\n";
	echo strpos($e->getMessage(), "must be of type int") !== false ? "names the type\n" : "untyped\n";
}

try {
	explode(",", "a,b", array());
} catch (Exception $e) {
	echo "wrong: Exception caught a TypeError\n";
} catch (Throwable $e) {
	echo "Exception does not catch a TypeError\n";
}

echo implode(",", explode(",", "a,b")), "\n";
echo "still running\n";
---
caught TypeError
names the argument
names the type
Exception does not catch a TypeError
a,b
still running
