name: memory limit enforcement
options:
  memory_limit: 1M
runner:
  php: false
description: >
  In-place array growth trips memory_limit and the error is catchable as
  RuntimeException, on both engines.
---
<?php

try {
    $a = [];
    for ($i = 0; $i < 100000; $i++) {
        $a[] = str_repeat("x", 1000);
    }
    echo "unreachable\n";
} catch (RuntimeException $e) {
    echo "caught: " . get_class($e) . "\n";
}
?>
---
caught: RuntimeException
