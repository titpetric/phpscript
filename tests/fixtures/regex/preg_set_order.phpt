name: PREG_SET_ORDER
description: >
  PREG_SET_ORDER transposes $matches from one entry per capture group to one
  entry per match, each holding the whole match followed by its groups.
---
<?php
preg_match_all("/(\w)(\d)/", "a1b2", $sets, PREG_SET_ORDER);
var_dump($sets);
preg_match_all("/(\w)(\d)/", "a1b2", $cols, PREG_PATTERN_ORDER);
var_dump($cols);
---
array(2) {
  [0]=>
  array(3) {
    [0]=>
    string(2) "a1"
    [1]=>
    string(1) "a"
    [2]=>
    string(1) "1"
  }
  [1]=>
  array(3) {
    [0]=>
    string(2) "b2"
    [1]=>
    string(1) "b"
    [2]=>
    string(1) "2"
  }
}
array(3) {
  [0]=>
  array(2) {
    [0]=>
    string(2) "a1"
    [1]=>
    string(2) "b2"
  }
  [1]=>
  array(2) {
    [0]=>
    string(1) "a"
    [1]=>
    string(1) "b"
  }
  [2]=>
  array(2) {
    [0]=>
    string(1) "1"
    [1]=>
    string(1) "2"
  }
}
