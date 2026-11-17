name: the go runtime keeps php's output buffers for echo
description: >
  ob_start() captures what echo writes and ob_get_clean() returns it and
  closes the buffer, nesting the way php nests it. The go runtime names no
  throwable class and still keeps these: a template captures a fragment and
  echoes it. php runs the body unchanged, so both columns define the output.
runtime: go
runner:
  php: true
---
<?php

ob_start();
echo "outer ";
ob_start();
echo "inner";
$inner = ob_get_clean();
echo $inner;
$outer = ob_get_clean();
echo "[", $outer, "]\n";
echo "level ", ob_get_level(), "\n";
---
[outer inner]
level 0
