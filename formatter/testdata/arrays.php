<?php
$one = array("name" => "Tit Petric");
$two = array("id" => 1, "name" => "Tit Petric");
$three = array("id" => 1, "name" => "Tit Petric", "active" => true);
$list = array(1, 2, 3, 4);
$long = array("prefix" => __DIR__ . "/../vendor/titpetric/minitpl/code/MiniTPL" . "/Compiler.php" . $suffix . $ext);
$empty = array();
$monthly = array(
	"labels" => array(),
	"datasets" => array(
		array("label" => "Minimum duration", "backgroundColor" => "#c7d2fe", "borderRadius" => 4, "data" => array()),
	),
);
$retval[] = array(
	"name" => isset($run["exitCode"]) ? "Exit: " . $run["exitCode"] : "OK",
	"date" => isset($run["stamp"]) ? $run["stamp"] : "",
	"value" => isset($run["duration"]) ? $run["duration"] : 0
);
render($monthly, array("a" => 1, "b" => 2, "c" => 3));
