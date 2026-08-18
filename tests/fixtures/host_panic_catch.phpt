name: host panic becomes a catchable exception
flatstack: true
description: >
  A panic raised inside a registered Go binding is converted into a PHP
  exception at the call boundary, so try/catch handles it and the script keeps
  running. invokeAny dispatches bindings either through a fast type switch or
  through reflection, and the boundary has to hold on both paths.

  host_panic_fast and host_panic_reflect are test bindings with no PHP
  counterpart, so php cannot run this source and the expected output is the
  runtime's contract rather than a comparison against PHP.
---
<?php

try {
	host_panic_fast("x");
	echo "fast: not reached\n";
} catch (Exception $e) {
	echo "fast: caught\n";
	echo "fast: message ok: " . (strpos("" . $e, "fast path exploded") !== false ? "yes" : "no") . "\n";
}

try {
	host_panic_reflect("x");
	echo "reflect: not reached\n";
} catch (Exception $e) {
	echo "reflect: caught\n";
	echo "reflect: message ok: " . (strpos("" . $e, "reflect path exploded") !== false ? "yes" : "no") . "\n";
}

echo "still running\n";
?>
---
fast: caught
fast: message ok: yes
reflect: caught
reflect: message ok: yes
still running
