name: a caught throwable answers the Throwable methods
flatstack: true
description: >
  throw propagates the object a script threw rather than a rendering of it, so
  a catch clause binds the instance and getMessage() and getCode() report what
  was constructed. Every throwable class name is one type here, so the class
  named by the catch does not narrow what it catches. The expected output is
  what php 8.4 prints for this source.
---
<?php

try {
	throw new Exception("boom", 42);
} catch (Throwable $e) {
	echo $e->getMessage() . "\n";
	echo $e->getCode() . "\n";
}

try {
	throw new RuntimeException("later");
} catch (Exception $e) {
	echo $e->getMessage() . "\n";
	echo $e->getCode() . "\n";
}

echo "still running\n";
?>
---
boom
42
later
0
still running
