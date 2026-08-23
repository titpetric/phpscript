name: combined preg flags
description: >
  Bitwise or combines preg flag constants the way PHP code writes them:
  PREG_OFFSET_CAPTURE | PREG_SET_ORDER reaches preg_match_all as 258, and
  each flag still takes effect.
---
<?php
echo PREG_OFFSET_CAPTURE | PREG_SET_ORDER, "\n";
preg_match_all("/(\w)(\d)/", "a1b2", $sets, PREG_OFFSET_CAPTURE | PREG_SET_ORDER);
var_dump($sets);
preg_match("/(\d+)/", "id 42", $one, PREG_OFFSET_CAPTURE | PREG_UNMATCHED_AS_NULL);
var_dump($one);
---
258
array(2) {
  [0]=>
  array(3) {
    [0]=>
    array(2) {
      [0]=>
      string(2) "a1"
      [1]=>
      int(0)
    }
    [1]=>
    array(2) {
      [0]=>
      string(1) "a"
      [1]=>
      int(0)
    }
    [2]=>
    array(2) {
      [0]=>
      string(1) "1"
      [1]=>
      int(1)
    }
  }
  [1]=>
  array(3) {
    [0]=>
    array(2) {
      [0]=>
      string(2) "b2"
      [1]=>
      int(2)
    }
    [1]=>
    array(2) {
      [0]=>
      string(1) "b"
      [1]=>
      int(2)
    }
    [2]=>
    array(2) {
      [0]=>
      string(1) "2"
      [1]=>
      int(3)
    }
  }
}
array(2) {
  [0]=>
  array(2) {
    [0]=>
    string(2) "42"
    [1]=>
    int(3)
  }
  [1]=>
  array(2) {
    [0]=>
    string(2) "42"
    [1]=>
    int(3)
  }
}
