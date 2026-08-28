name: a function static persists across calls
description: >
  `static $n = 0;` initializes once per function lifetime and later writes
  persist, so three calls count 1, 2, 3 rather than re-running the
  initializer. Verified against php 8.5.
---
<?php

function counter() {
	static $n = 0;
	$n++;
	echo $n, "\n";
}

counter();
counter();
counter();
---
1
2
3
