name: user function declaration and call
description: >
  Test that user function declarations, argument binding, return values, and
  calls compile to and execute with flat bytecode.
---
<?php
function add($a, $b) {
	return $a + $b;
}

$res = add(15, 27);
echo $res;
?>
---
42
