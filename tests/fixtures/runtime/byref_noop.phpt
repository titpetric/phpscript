name: reference markers parse and bind by value
description: >
  `$a = &$b` and `function &getRef()` are documented won't-implements
  (docs/design.md): both parse, the formatter keeps them, and the runtime
  binds and returns by value, so no aliasing happens. php would print 9 and
  10 through the references, so the php runner is opted out. phpscript lint
  reports each marker.
runner:
  php: false
---
<?php

function &getRef() {
	static $x = 5;
	return $x;
}

$b = 2;
$a = &$b;
$b = 9;
echo $a, "\n";

$r = &getRef();
$r = 10;
echo getRef(), "\n";
---
2
5
