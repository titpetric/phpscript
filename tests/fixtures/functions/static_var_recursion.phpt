name: recursive calls share one static binding
description: >
  Every activation of a function reads and writes the same static storage, so
  the maximum a deeper recursive call records is visible in the shallower
  frames after the inner call returns. Verified against php 8.5.
---
<?php

function descend($depth) {
	static $max = 0;
	if ($depth > $max) {
		$max = $depth;
	}
	if ($depth < 3) {
		descend($depth + 1);
	}
	echo "at ", $depth, " max ", $max, "\n";
}

descend(1);
---
at 3 max 3
at 2 max 3
at 1 max 3
