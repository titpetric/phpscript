<?php

function &getRef() {
	static $x = 5;
	return $x;
}

class Box {
	public function &value() {
		return 7;
	}
}

$b = 2;
$a = &$b;
$row = &$table["users"];
$f = function &() use (&$b) {
	return $b;
};
echo $a, "\n";
