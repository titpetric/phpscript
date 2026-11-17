name: a catch clause with no type binds the go error
description: >
  Under the go runtime `catch ($e)` is the implicit form of `catch (error $e)`
  and binds the thrown error. The clause keeps its spelling through the
  formatter. php parses neither the import nor the clause, and only phpscript
  defines the expected output.
runtime: go
---
<?php

import "errors";

try {
    throw errors.New("implicit");
} catch ($e) {
    echo $e, "\n";
}
---
implicit
