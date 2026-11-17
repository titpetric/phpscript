name: getenv and numcpu read the environment and the cpu count
description: >
  The go runtime answers getenv with the variable's value, null for a name the
  process does not hold, and numcpu from runtime.NumCPU. The ?? operator
  supplies the default, and int() of null is the zero value the test server
  reads its worker count from. php names neither function under this runtime,
  so phpscript defines the expected output.
runtime: go
---
<?php

echo getenv("PHPS_GO_RUNTIME_TEST_UNSET_9C1") . "|\n";
echo getenv("PHPS_GO_RUNTIME_TEST_UNSET_9C1") ?? "default\n";
echo numcpu() >= 1 ? "cpus\n" : "none\n";
echo int(getenv("PHPS_GO_RUNTIME_TEST_UNSET_9C1")) . "\n";
---
|
default
cpus
0
