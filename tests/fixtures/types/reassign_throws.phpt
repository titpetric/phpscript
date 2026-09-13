name: type reassignment is a RuntimeException
description: >
  A deliberate divergence from PHP (docs/README.md, known divergences): the
  first non-null value assigned to a variable declares its type, and a later
  assignment of another type throws a RuntimeException. PHP runs the same
  program and lets $a become an int, which is why the php runner is opted
  out. The linter reports the same line as a fatal finding before the
  program runs.
runner:
  php: false
error: "no reassignment: $a previously declared as string"
---
<?php

$a = "text";
echo "declared\n";
$a = 1;
echo "unreachable\n";
---
Internal Server Error
