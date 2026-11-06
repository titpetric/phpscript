name: a class is reachable through the binding area that installs it
description: >
  A class answers under the area that installs it, the way a function does. A
  class spelled like the area's last segment takes the area itself, so the
  Database binding is \Runtime\PS\Database and Database\Migrate beside it is
  \Runtime\PS\Database\Migrate. The exception and Error classes have areas of
  their own, Runtime\PS\Exceptions and Runtime\PS\Errors. get_class reports the
  name the binding registered under, which is the name php uses, and the bare
  spelling constructs the same class. php has no Runtime namespace, so only
  phpscript defines the expected output.
runner:
  php: false
---
<?php

var_dump(class_exists('Runtime\PS\Database'));
var_dump(class_exists('Runtime\PS\Database\Migrate'));
var_dump(class_exists('runtime\ps\exceptions\RUNTIMEEXCEPTION'));
var_dump(class_exists('Runtime\PS\Errors\TypeError'));
var_dump(class_exists('Runtime\PS\Database\Missing'));
var_dump(class_exists('Runtime\PS\Nope\Database'));

$qualified = new \Runtime\PS\Exceptions\RuntimeException("through the area", 7);
echo get_class($qualified), "|", $qualified->getMessage(), "|", $qualified->getCode(), "\n";

try {
	throw new \Runtime\PS\Errors\TypeError("caught bare", 0);
} catch (TypeError $caught) {
	echo get_class($caught), "|", $caught->getMessage(), "\n";
}
---
bool(true)
bool(true)
bool(true)
bool(true)
bool(false)
bool(false)
RuntimeException|through the area|7
TypeError|caught bare
