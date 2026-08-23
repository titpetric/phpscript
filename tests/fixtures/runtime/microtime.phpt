name: time and microtime
description: >
  time() returns an integer epoch, microtime(true) returns float seconds that
  agree with it, and the argumentless microtime() keeps the "msec sec" string
  shape. Values change per run, so the fixture asserts properties.
---
<?php

$t = time();
echo ($t > 1500000000) ? "time_int\n" : "time_bad\n";

$m = microtime(true);
echo (is_numeric($m) && !is_int($m)) ? "micro_float\n" : "micro_not_float\n";
echo ($m - $t < 5 && $t - $m < 5) ? "micro_agrees\n" : "micro_disagrees\n";

$s = microtime();
$parts = explode(" ", $s);
echo (count($parts) == 2) ? "micro_string_pair\n" : "micro_string_bad\n";

$a = microtime(true);
$b = microtime(true);
echo ($b >= $a) ? "monotonic\n" : "backwards\n";
?>
---
time_int
micro_float
micro_agrees
micro_string_pair
monotonic
