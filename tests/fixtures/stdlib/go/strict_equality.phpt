name: the go runtime refuses ===
description: >
  go compares with == and != alone. A file that writes === or !== fails the
  parse under the go runtime, before the body runs.
runtime: go
error: the go runtime compares with == and !=
---
<?php

$a = 1;
echo $a === 1 ? "yes" : "no";
---
Internal Server Error
