name: an undefined constant raises RuntimeException
description: >
  A bare name nothing defines throws. php raises Error; this raises
  RuntimeException, so catch (Exception) takes it. An unset variable of the
  same spelling stays null, which is the pair this separates. php cannot run
  the fixture, since the class is the divergence.
runner:
  php: false
---
<?php

define("DEFINED_ONE", 1);
echo DEFINED_ONE, "\n";

// An unset variable is null and keeps running.
echo "[", $missing, "]\n";

try {
	echo MISSING_ONE;
} catch (RuntimeException $e) {
	echo get_class($e), ": ", $e->getMessage(), "\n";
}

// An Exception clause takes it, which is the break from php: there an
// undefined constant is an Error and this clause would not match.
try {
	echo MISSING_TWO;
} catch (Exception $e) {
	echo "exception\n";
}

// defined() answers without raising.
var_dump(defined("MISSING_THREE"));
var_dump(defined("DEFINED_ONE"));
---
1
[]
RuntimeException: Undefined constant "MISSING_ONE"
exception
bool(false)
bool(true)
