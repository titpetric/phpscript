name: PREG_OFFSET_CAPTURE
description: >
  PREG_OFFSET_CAPTURE turns every entry into a pair of the matched text and its
  offset. The offset is a byte offset even under the u modifier, so a match after
  a two-byte character starts at 2 rather than 1.
---
<?php
preg_match_all("/\{([a-z]+)\}/", "a{one}b{two}", $m, PREG_OFFSET_CAPTURE);
var_dump(is_array($m[0][0]));
var_dump($m[0][0]);
var_dump($m[1][1]);
preg_match("/b/u", "\xc3\xa4bc", $mu, PREG_OFFSET_CAPTURE);
var_dump($mu[0]);
---
bool(true)
array(2) {
  [0]=>
  string(5) "{one}"
  [1]=>
  int(1)
}
array(2) {
  [0]=>
  string(3) "two"
  [1]=>
  int(8)
}
array(2) {
  [0]=>
  string(1) "b"
  [1]=>
  int(2)
}
