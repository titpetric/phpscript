name: fmt.Errorf formats an error and a wrap is caught as its inner error
description: >
  fmt.Errorf returns a go error whose message is the format applied to its
  arguments, and a %w argument wraps one error in another. The catch binds the
  error the statement threw, wrapper included, as go's `if err != nil` reads
  it. php has no go fmt package and only phpscript defines the expected
  output.
runtime: go
---
<?php

import "fmt";

try {
    throw fmt.Errorf("failed: %d", 42);
} catch (error $e) {
    echo $e, "\n";
}

$cause = fmt.Errorf("boom");
try {
    throw fmt.Errorf("wrap: %w", $cause);
} catch (error $e) {
    echo $e, "\n";
}
---
failed: 42
wrap: boom
