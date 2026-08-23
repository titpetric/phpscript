name: calling an undefined function is thrown and catchable
description: >
  A call to a function no engine knows reports the same condition on both, so
  the message does not depend on which engine ran the script. PHP spells it
  "Call to undefined function name()"; only the part both spell the same way is
  compared, along with the name of the function that was called.
---
<?php

try {
	no_such_function_here();
	echo "not reached\n";
} catch (Throwable $e) {
	echo "caught\n";
	echo strpos($e->getMessage(), "undefined function") !== false ? "names the condition\n" : "other\n";
	echo strpos($e->getMessage(), "no_such_function_here") !== false ? "names the function\n" : "unnamed\n";
}

echo "still running\n";
?>
---
caught
names the condition
names the function
still running
