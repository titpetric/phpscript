name: magic path constants
description: >
  __FILE__, __DIR__ and __LINE__ are resolved when a file is compiled, so none
  of them is a constant a script can ask about. The fixture asserts the
  directory name rather than the file name, because the php runner executes a
  temporary copy of the source.
---
<?php

echo basename(__DIR__), "\n";
echo __FILE__ !== "" ? "FILE set\n" : "FILE empty\n";
echo __DIR__ !== "" ? "DIR set\n" : "DIR empty\n";

// __LINE__ is the line it is written on, not the line of the call it is
// passed to.
echo __LINE__, "\n";
echo strlen("x"), ":", __LINE__, "\n";

function where() {
	return __LINE__;
}
echo where(), "\n";

// php resolves all three when it compiles, so none of them is a constant.
var_dump(defined("__FILE__"));
var_dump(defined("__DIR__"));
var_dump(defined("__LINE__"));
$c = get_defined_constants();
var_dump(array_key_exists("__FILE__", $c));
var_dump(array_key_exists("__LINE__", $c));
---
paths
FILE set
DIR set
9
1:10
13
bool(false)
bool(false)
bool(false)
bool(false)
bool(false)
