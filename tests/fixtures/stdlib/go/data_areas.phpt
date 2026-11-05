name: Runtime\Go slices, maps and JSON areas work over php values
description: >
  `Slices` and `Maps` read php arrays in order and answer new ones, and
  `Encoding\JSON` encodes and decodes through the runtime's own json code.
  php has no Runtime namespace, so only phpscript defines the expected output.
runner:
  php: false
---
<?php

use Runtime\Go\Encoding\JSON;
use Runtime\Go\Maps;
use Runtime\Go\Slices;

$in = [3, 1, 2];
echo implode(",", Slices\Sort($in)), " ", implode(",", $in), " ", Slices\Max($in), "\n";
var_dump(Slices\Contains([1, 2], "2"));
echo implode(",", Maps\Keys(["b" => 1, "a" => 2])), "\n";
echo JSON\Marshal(Maps\Copy(["a" => 1], ["b" => [true]])), "\n";
echo JSON\Unmarshal('{"x":[1,2]}')["x"][1], "\n";
---
1,2,3 3,1,2 3
bool(false)
b,a
{"a":1,"b":[true]}
2
