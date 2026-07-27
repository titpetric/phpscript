name: deferred function usage
description: >
  A deferred function runs when the current file finishes, after the rest of
  the file has produced its output.
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
