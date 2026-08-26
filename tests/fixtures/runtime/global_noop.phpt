name: global parses and binds nothing
description: >
  The global statement is a documented won't-implement (docs/design.md): it
  parses and the variable stays unset, so a port keeps loading and the docs
  say what to do with the line. php would import the binding and print
  "sees x", so the php runner is opted out.
runner:
  php: false
---
<?php

$x = 1;
function f() {
	global $x;
	echo isset($x) ? "sees x" : "global is a no-op", "\n";
}
f();
---
global is a no-op
