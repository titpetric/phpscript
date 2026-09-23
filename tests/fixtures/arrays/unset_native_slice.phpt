name: unset removes an element of a native slice
description: >
  unset($list[$i]) on a slice a binding returned removes the element: count
  drops, the value is gone and the ones around it stay. explode() returns such
  a slice. Only what php and phpscript agree on is asserted here, which is
  count, implode and array_values; the key numbering after the removal differs
  and unset_native_slice_reindexes.phpt covers that.
---
<?php

$e = explode(",", "x,y,z");
var_dump(count($e));

unset($e[1]);
var_dump(count($e));
echo implode(",", $e), "\n";
echo implode(",", array_values($e)), "\n";

// An index that was never there changes nothing.
unset($e[99]);
var_dump(count($e));

// Removing the first element leaves the rest.
unset($e[0]);
echo implode(",", array_values($e)), "\n";
var_dump(count($e));

// Emptying it is allowed.
$one = explode(",", "only");
unset($one[0]);
var_dump(count($one));
var_dump(empty($one));
?>
---
int(3)
int(2)
x,z
x,z
int(2)
z
int(1)
int(0)
bool(true)
