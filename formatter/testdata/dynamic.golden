<?php

class Logger {
	public function info($msg) {
		return "log: " . $msg;
	}
}

$className = "Logger";
$obj = new $className;
$other = new $className("arg");
$method = "info";
echo $obj->$method("dynamic"), "\n";
echo $obj->$method(1, 2), "\n";
