name: preg_match flags and offset
description: >
  preg_match takes $flags and $offset. The offset moves where matching starts
  without changing where the subject begins, so an anchor and a word boundary
  still see the real start of the string. An offset past the end is false.
---
<?php
var_dump(preg_match("/b/", "abcb", $m, PREG_OFFSET_CAPTURE, 2));
var_dump($m[0]);
var_dump(preg_match("/^b/", "abcb", $m2, 0, 1));
var_dump(preg_match("/\bb/", "ab cb", $m3, 0, 1));
var_dump(preg_match("/b/", "ab", $m4, 0, 99));
---
int(1)
array(2) {
  [0]=>
  string(1) "b"
  [1]=>
  int(3)
}
int(0)
int(0)
bool(false)
