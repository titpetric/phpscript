name: memory alias dedup
runner:
  php: false
description: >
  Two variables holding the same array count it once in memory_get_usage.
  (phpscript arrays are shared by pointer; PHP would copy on write, so the
  fixture opts out of the php runner.)
---
<?php

$a = [];
for ($i = 0; $i < 100; $i++) {
    $a[] = str_repeat("z", 1000);
}
$u1 = memory_get_usage();
$b = $a;
$u2 = memory_get_usage();

echo ($u2 - $u1 < 1000) ? "deduped\n" : "double counted: " . ($u2 - $u1) . "\n";
?>
---
deduped
