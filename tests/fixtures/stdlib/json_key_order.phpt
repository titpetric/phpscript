name: json_encode preserves key order
description: >
  json_encode writes an array in the order it was built rather than in key
  order, so an encoded row reads back the way the script assembled it. A dense
  list encodes as a JSON array and carries no keys, and the order survives a
  decode and a re-encode.
---
<?php

// json_encode writes an array in the order it was built, not in key order.
echo json_encode(array(2 => "c", 1 => "b", 0 => "a")), "\n";
echo json_encode(array("z" => 1, "m" => 2, "a" => 3)), "\n";
echo json_encode(array("x" => array("q" => 1, "b" => 2))), "\n";

// A dense list is a JSON array, so it carries no keys at all.
echo json_encode(array(1, 2, 3)), "\n";
echo json_encode(array()), "\n";

// Order survives a decode and a re-encode.
echo json_encode(json_decode('{"b":1,"a":2}', true)), "\n";

// Scalars and nesting.
echo json_encode(array("q" => null, "w" => true, "e" => 1.5)), "\n";
echo json_encode(array("nested" => array("deep" => array("k" => "v")))), "\n";
---
{"2":"c","1":"b","0":"a"}
{"z":1,"m":2,"a":3}
{"x":{"q":1,"b":2}}
[1,2,3]
[]
{"b":1,"a":2}
{"q":null,"w":true,"e":1.5}
{"nested":{"deep":{"k":"v"}}}
