name: memory accounting and get_usage
runner:
  php: false
description: >
  memory_get_usage reports request-scoped memory allocations and tracks variable mutations.
---
<?php

$m0 = memory_get_usage();
$sample = "some sample text for testing allocations";
$m1 = memory_get_usage();

echo ($m1 >= $m0) ? "m1_valid\n" : "m1_invalid\n";

unset($sample);
$m2 = memory_get_usage();

echo ($m2 <= $m1) ? "m2_valid\n" : "m2_invalid\n";

try {
    throw new RuntimeException("runtime failure", 1);
} catch (RuntimeException $e) {
    echo "caught runtime: " . get_class($e) . "\n";
}

try {
    throw new RuntimeException("generic failure", 2);
} catch (Exception $e) {
    echo "caught exception base: " . get_class($e) . "\n";
}
?>
---
m1_valid
m2_valid
caught runtime: RuntimeException
caught exception base: RuntimeException
