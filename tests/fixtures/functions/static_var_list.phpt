name: a static declaration list binds every name
description: >
  `static $count = 0, $label;` declares both names in one statement: the
  first with an initializer, the second defaulting to null but still bound to
  persistent storage, so the write a call makes to it survives into the next
  call. Verified against php 8.5.
---
<?php

function tally() {
	static $count = 0, $label;
	$count++;
	$label = "run " . $count;
	echo $count, " ", $label, "\n";
	echo isset($label) ? "label set" : "label unset", "\n";
}

tally();
tally();
---
1 run 1
label set
2 run 2
label set
