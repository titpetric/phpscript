name: memory frame release
runner:
  php: false
description: >
  A returned function's locals stop counting toward memory_get_usage, so
  repeated calls do not accumulate.
---
<?php

function eat()
{
    $x = str_repeat("a", 100000);
    return 1;
}

$m0 = memory_get_usage();
for ($i = 0; $i < 100; $i++) {
    eat();
}
$m1 = memory_get_usage();

echo ($m1 - $m0 < 10000) ? "released\n" : "leaked: " . ($m1 - $m0) . "\n";
?>
---
released
