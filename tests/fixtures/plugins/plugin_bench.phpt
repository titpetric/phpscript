name: go plugin call overhead
description: >
  Bench comes from a Go plugin loaded by filename. The counter shows every call
  reached the plugin, and the per-call cost is asserted as a bound because
  timings vary per run; the measured figure goes to the log. The class comes
  from a .so, so php cannot run this source.
plugins: ../../testdata/plugins/basic/plugin.so
runner:
  php: false
---
<?php

$n = 1000;
$b = new Bench();

$start = microtime(true);
for ($i = 0; $i < $n; $i++) {
    $b->count();
}
$elapsed = microtime(true) - $start;

// The counter is the proof every iteration crossed into the plugin: a loop
// that was optimised away, or a method that resolved to something else, does
// not leave the count at $n.
echo ($b->count() == $n + 1) ? "crossings\n" : "crossings_lost\n";

echo ($elapsed >= 0) ? "measured\n" : "clock_backwards\n";
echo (($elapsed / $n) < 0.001) ? "under_1ms_per_op\n" : "slow\n";
echo ($b->echo("round trip") == "round trip") ? "echo_ok\n" : "echo_bad\n";

$b->report($n, $elapsed);
?>
---
crossings
measured
under_1ms_per_op
echo_ok
