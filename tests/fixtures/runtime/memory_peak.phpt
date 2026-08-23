name: memory peak usage
runner:
  php: false
description: >
  memory_get_peak_usage keeps the high-water mark after the allocation that
  set it is released.
---
<?php

$big = str_repeat("c", 100000);
$u = memory_get_usage();
unset($big);
$peak = memory_get_peak_usage();
$now = memory_get_usage();

echo ($peak >= $u) ? "peak_holds\n" : "peak_dropped\n";
echo ($now < $u) ? "usage_dropped\n" : "usage_held\n";
?>
---
peak_holds
usage_dropped
