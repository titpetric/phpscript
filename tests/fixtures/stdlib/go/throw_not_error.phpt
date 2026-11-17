name: throw refuses a value that is not an error
description: >
  throw takes an error under the go runtime. A string is not one, so the
  statement raises the same TypeError a binding's parameter check raises, and
  the catch clause declaring error handles it. php would render the string,
  so only phpscript defines the expected output.
runtime: go
---
<?php

try {
    throw "boom";
} catch (error $e) {
    echo $e, "\n";
}
---
throw(): Argument #1 must be of type error, string given
