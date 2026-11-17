name: the runtime option reads every file as an import of the go namespace
description: >
  runner.runtime: go installs Runtime\Go and no area under Runtime\PS or
  Runtime\PX, and the parser reads a file as though it wrote use Runtime\Go;.
  The fixture writes an import and the go builtins with no use statement, and
  php cannot parse either, so only phpscript defines the expected output.
runtime: go
---
<?php

import "strings";

$parts = strings.Split("go,runtime", ",");
$empty = make([]string, 0);

echo strings.ToUpper($parts[0]), " ", len($parts), " ", len($empty), "\n";
---
GO 2 0
