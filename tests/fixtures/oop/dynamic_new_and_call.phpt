name: a variable names the class of new and the method of a call
description: >
  `new $className` instantiates the class the variable names — a PHP class
  or a registered Go constructor alike — and `$obj->$method(...)` calls the
  method a variable names, including a name built at run time. Verified
  against php 8.5.
---
<?php

class Logger {
	public $prefix = "log";

	public function info($msg) {
		return $this->prefix . ": " . $msg;
	}

	public function warn($msg) {
		return strtoupper($this->prefix) . "! " . $msg;
	}
}

$className = "Logger";
$obj = new $className();
echo get_class($obj), "\n";

foreach (array("info", "warn") as $method) {
	echo $obj->$method("dynamic dispatch"), "\n";
}

$level = "in";
$call = $level . "fo";
echo $obj->$call("built name"), "\n";

$ctor = "Exception";
$err = new $ctor("registered constructor");
echo get_class($err), ": ", $err->getMessage(), "\n";
---
Logger
log: dynamic dispatch
LOG! dynamic dispatch
log: built name
Exception: registered constructor
