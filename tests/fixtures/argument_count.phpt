name: a surplus argument is thrown and catchable
flatstack: true
description: >
  Passing more arguments than a binding declares is refused rather than
  silently ignored, and the refusal is a throwable a script can catch, so
  execution continues after the catch block. PHP raises ArgumentCountError for
  the same call and names the function in the message; both are checked here
  against php 8.4, and the wording of the two messages differs, so only the
  function name is compared.
---
<?php

try {
	strlen("a", "b");
	echo "not reached\n";
} catch (Throwable $e) {
	echo "caught\n";
	echo strpos($e->getMessage(), "strlen()") !== false ? "names the function\n" : "unnamed\n";
}

echo strlen("abc"), "\n";
echo substr("abcdef", 2), "\n";
echo "still running\n";
?>
---
caught
names the function
3
cdef
still running
