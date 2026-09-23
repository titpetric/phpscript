name: unset on a native slice renumbers the keys it leaves
description: >
  php leaves a hole where an element was and keeps the keys around it, so
  array_keys() answers 0,2 and json_encode() writes an object. A Go slice is
  dense and holds neither, so phpscript renumbers, as array_values() does. The
  php runner is opted out because that is the divergence being stated; the two
  Go engines agree with each other and docs/README.md records it. A script
  array keeps php's numbering, which is the second half of this.
runner:
  php: false
---
<?php

$e = explode(",", "x,y,z");
unset($e[1]);

echo implode(",", array_keys($e)), "\n";
echo json_encode($e), "\n";
var_dump(isset($e[2]));
var_dump($e[1]);

// A script array is unchanged: the hole and the keys around it are php's.
$a = array("x", "y", "z");
unset($a[1]);
echo implode(",", array_keys($a)), "\n";
echo json_encode($a), "\n";
var_dump(isset($a[2]));
?>
---
0,1
["x","z"]
bool(false)
string(1) "z"
0,2
{"0":"x","2":"z"}
bool(true)
