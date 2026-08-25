name: array_key_exists
description: >
  array_key_exists reports whether a key is present, which is true even when
  the value stored there is null, and a decimal string names the same key the
  integer does.
---
<?php
$row = array("id" => 7, "name" => null);

// The whole point of array_key_exists: a key holding null still exists,
// where isset() reports it as absent.
var_dump(array_key_exists("name", $row));
var_dump(isset($row["name"]));
var_dump(array_key_exists("missing", $row));

// A decimal string names the same key the integer does, in both directions.
$list = array(10, 20, 30);
var_dump(array_key_exists("1", $list));
var_dump(array_key_exists(1, $list));
var_dump(array_key_exists("3", $list));

$mixed = array("7" => "seven", "x" => null);
var_dump(array_key_exists(7, $mixed));
var_dump(array_key_exists("7", $mixed));
var_dump(array_key_exists("x", $mixed));
var_dump(isset($mixed["x"]));
---
bool(true)
bool(false)
bool(false)
bool(true)
bool(true)
bool(false)
bool(true)
bool(true)
bool(true)
bool(false)
