name: analytics ring buffer records
description: >
  A script records requests through the namespaced Analytics bindings. Counts
  are asserted relative to the count on entry, because the harness reuses one
  runtime across runs, and the elapsed time as a bound because timings vary.
  The bindings have no PHP counterpart, so php cannot run this source.
runner:
  php: false
---
<?php

$before = Analytics\count();

Analytics\record("/index.php", 200, 1234);
echo (Analytics\count() == $before + 1) ? "recorded\n" : "not_recorded\n";

$last = Analytics\last();
echo ($last->route == "/index.php") ? "route_ok\n" : "route_bad\n";
echo ($last->status == 200) ? "status_ok\n" : "status_bad\n";

$t0 = microtime(true);
for ($i = 0; $i < 1000; $i++) {
    Analytics\record("/index.php", 200, $i);
}
$elapsed = microtime(true) - $t0;

echo (Analytics\count() == $before + 1001) ? "count_ok\n" : "count_bad\n";
echo ($elapsed >= 0 && $elapsed < 5) ? "bounded\n" : "slow\n";

fwrite(STDERR, "analytics: 1000 calls in " . $elapsed . "s\n");
?>
---
recorded
route_ok
status_ok
count_ok
bounded
