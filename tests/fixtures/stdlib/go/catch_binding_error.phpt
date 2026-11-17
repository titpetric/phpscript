name: a go binding's returned error reaches catch (error)
description: >
  A go binding that fails returns an error, and the call raises it. The go
  runtime's catch clause declaring error handles it, so a script wraps a
  binding call the way go wraps a call returning a non-nil error. The bound
  value is the error the binding returned, so the message keeps its
  strconv.Atoi prefix. php has no strconv area and only phpscript defines the
  expected output.
runtime: go
---
<?php

import "strconv";

try {
    $n = strconv.Atoi("x");
} catch (error $e) {
    echo $e, "\n";
}
---
strconv.Atoi: parsing "x": invalid syntax
