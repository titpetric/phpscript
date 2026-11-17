name: getenv and numcpu read the environment and the cpu count
description: >
  The go runtime answers getenv from os.Getenv, an empty string for a name the
  process does not hold, and numcpu from runtime.NumCPU. int() of that empty
  string is the zero value, and the test server reads its worker count this
  way. php names neither function under this runtime, so phpscript defines the
  expected output.
runtime: go
---
<?php

echo getenv("PHPS_GO_RUNTIME_TEST_UNSET_9C1") . "|\n";
echo numcpu() >= 1 ? "cpus\n" : "none\n";
echo int(getenv("PHPS_GO_RUNTIME_TEST_UNSET_9C1")) . "\n";
---
|
cpus
0
