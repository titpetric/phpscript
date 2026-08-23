name: str_pad, str_split and strrev
description: >
  str_pad truncates a multi-character pad to fit and splits an odd remainder to
  the right under STR_PAD_BOTH; a target shorter than the subject is a no-op.
  str_split leaves a short final chunk, and returns an empty array for an empty
  string. strrev reverses bytes, not characters.
---
<?php
var_dump(str_pad("ab", 7, "xy", STR_PAD_BOTH));
var_dump(str_pad("7", 3, "0", STR_PAD_LEFT));
var_dump(str_pad("7", 3, "0"));
var_dump(str_pad("abc", 2, "x"));
var_dump(str_split("abcde", 2));
var_dump(str_split(""));
var_dump(str_split("abc"));
var_dump(strrev("abc"));
---
string(7) "xyabxyx"
string(3) "007"
string(3) "700"
string(3) "abc"
array(3) {
  [0]=>
  string(2) "ab"
  [1]=>
  string(2) "cd"
  [2]=>
  string(1) "e"
}
array(0) {
}
array(3) {
  [0]=>
  string(1) "a"
  [1]=>
  string(1) "b"
  [2]=>
  string(1) "c"
}
string(3) "cba"
