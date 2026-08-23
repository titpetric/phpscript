name: deferred function usage
runner:
  php: false
description: >
  A deferred function runs when the current file finishes, after the rest of
  the file has produced its output. defer() is a phpscript extension, so there
  is no PHP behavior to compare against.
---
<?php

defer(function() {
    echo "Expected result\n";
});

echo "Before deferred function\n";

?>
---
Before deferred function
Expected result
