name: preg_split
description: >
  preg_split honours $limit, and its flags decide whether the captured
  delimiters are interleaved into the result and whether empty pieces survive.
---
<?php
var_dump(preg_split("/(,)/", "a,b", -1, PREG_SPLIT_DELIM_CAPTURE));
var_dump(preg_split("/,/", "a,b,c", 2));
var_dump(preg_split("/,/", "a,,b", -1, PREG_SPLIT_NO_EMPTY));
var_dump(preg_split("/,/", "a,b"));
---
array(3) {
  [0]=>
  string(1) "a"
  [1]=>
  string(1) ","
  [2]=>
  string(1) "b"
}
array(2) {
  [0]=>
  string(1) "a"
  [1]=>
  string(3) "b,c"
}
array(2) {
  [0]=>
  string(1) "a"
  [1]=>
  string(1) "b"
}
array(2) {
  [0]=>
  string(1) "a"
  [1]=>
  string(1) "b"
}
