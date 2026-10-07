name: dollar-brace interpolation reads the variable form
description: >
  PHP's string grammar reads `${name}` and `${name[subscript]}` as the variable and
  its subscript, the same as `$name` and `$name[subscript]`. `\${name}` and a
  single-quoted literal interpolate nothing.

  The `${expression}` form names the variable whose name the expression returns and
  needs `$$name`, which is not implemented; the parser reports it.
---
<?php

$name = "Ada";
$arr = array("k" => "v", 2 => "two");
$i = 2;
$vv = "name";

echo "bare       |${name}\n";
echo "subscript  |${arr['k']}\n";
echo "int key    |${arr[2]}\n";
echo "var key    |${arr[$i]}\n";
echo "value of vv|${vv}\n";
echo "surrounded |x${name}y\n";
echo "escaped    |\${name}\n";
echo "single     |", '${name}', "\n";
echo "both forms |${name} {$name} $name\n";
---
bare       |Ada
subscript  |v
int key    |two
var key    |two
value of vv|name
surrounded |xAday
escaped    |${name}
single     |${name}
both forms |Ada Ada Ada
