name: interface constants resolve through the interface name
description: >
  An interface holds its constants under its own name, the same way a class
  holds class constants. Hook::POSITION_PRE reads the declaration directly;
  nothing is instantiated and no member moves anywhere. The constants work as
  array keys, in echo, and beside Hook::class.
---
<?php

interface Hook {
	const POSITION_PRE = "pre";
	const POSITION_POST = "post";
}

$hooks = array(Hook::POSITION_PRE => array(), Hook::POSITION_POST => array());
foreach ($hooks as $position => $list) {
	echo $position, "\n";
}
echo Hook::POSITION_PRE, ",", Hook::POSITION_POST, "\n";
echo Hook::class, "\n";
---
pre
post
pre,post
Hook
