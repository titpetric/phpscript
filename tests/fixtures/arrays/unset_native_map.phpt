name: unset removes a key from a native map
description: >
  unset($m[$k]) on a map a binding returned removes the key, the way it does
  on a script array: isset answers false and count drops. A key that is not
  there stays a no-op. get_defined_functions() returns such a map, keyed
  internal and user, and only the shape is asserted because the names differ
  per build.
---
<?php

$f = get_defined_functions();
var_dump(count($f));
var_dump(isset($f["internal"]));

unset($f["user"]);
var_dump(isset($f["user"]));
var_dump(count($f));

// A key that was never there changes nothing.
unset($f["absent"]);
var_dump(count($f));

// The key left behind still reads.
var_dump(is_array($f["internal"]));

// A script array answers the same way, which is the point.
$a = array("one" => 1, "two" => 2);
unset($a["one"]);
var_dump(count($a));
var_dump(isset($a["one"]));
?>
---
int(2)
bool(true)
bool(false)
int(1)
int(1)
bool(true)
int(1)
bool(false)
