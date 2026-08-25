name: the array union operator
description: >
  `+` on two arrays is their union: every key of the left operand, then the
  keys of the right operand the left does not already hold. The left entry wins
  where both hold a key, which is what makes it the way to apply defaults.
---
<?php

$defaults = array("host" => "localhost", "port" => 8080);
$given = array("port" => 9000, "tls" => true);

var_dump($defaults + $given);
var_dump($given + $defaults);
var_dump(array(1, 2) + array(9, 8, 7));
var_dump(array() + $defaults);
---
array(3) {
  ["host"]=>
  string(9) "localhost"
  ["port"]=>
  int(8080)
  ["tls"]=>
  bool(true)
}
array(3) {
  ["port"]=>
  int(9000)
  ["tls"]=>
  bool(true)
  ["host"]=>
  string(9) "localhost"
}
array(3) {
  [0]=>
  int(1)
  [1]=>
  int(2)
  [2]=>
  int(7)
}
array(2) {
  ["host"]=>
  string(9) "localhost"
  ["port"]=>
  int(8080)
}
