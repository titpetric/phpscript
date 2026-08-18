name: Array splice tests
description: Mainly tests array_splice.
---
<?php

function dump($a) {
	echo json_encode($a);
	echo "\n";
}

$input = array("red", "green", "blue", "yellow");
array_splice($input, 2);
dump($input);

$input = array("red", "green", "blue", "yellow");
array_splice($input, 1, -1);
dump($input);

$input = array("red", "green", "blue", "yellow");
array_splice($input, 1, count($input), "orange");
dump($input);

$input = array("red", "green", "blue", "yellow");
array_splice($input, -1, 1, array("black", "maroon"));
dump($input);

?>
---
["red","green"]
["red","yellow"]
["red","orange"]
["red","green","blue","black","maroon"]
