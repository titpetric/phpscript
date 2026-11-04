name: go slices
runner:
  php: false
description: >
  `use Runtime\Go` binds go's builtins and make allocates a go slice: the
  length and the capacity are the ones it was given, append fills it, and a
  foreach reads it back by index. php rejects the file, which writes a type
  where php writes an expression, so no php run can answer for this output.
---
<?php

use Runtime\Go;

$s = make([]string, 0, 3);
echo len($s), ",", cap($s), "\n";

$s = append($s, "a", "b");
echo len($s), ",", cap($s), "\n";
foreach ($s as $i => $v) {
    echo $i, "=", $v, ";";
}
echo "\n";

// A slice of a length holds the element type's zero value.
foreach (make([]int64, 2) as $v) {
    echo $v, ";";
}
echo "\n";

// A slice of a sized width holds typed values: the zero value carries the
// width, an element written to it narrows to it, and `[]uint8` is `[]byte`,
// which is one type in go and php's string here.
$w = make([]int8, 2);
$w[1] = 300;
var_dump($w);
var_dump(append(make([]uint8, 0), 104, 105));

// A nested type composes, and the inner slice is the zero value of a slice,
// which is the empty one.
foreach (make([][]int, 1) as $v) {
    echo len($v), ";";
}
echo "\n";

// php owns the name copy, so go's is the qualified one. It stops at the
// shorter of the two, and clear keeps a slice's length.
$dst = make([]string, 2);
echo Go\copy($dst, ["a", "b", "c"]), ":";
foreach ($dst as $v) {
    echo $v;
}
echo "\n";

clear($dst);
foreach ($dst as $v) {
    echo "[", $v, "]";
}
echo "\n";
---
0,3
2,3
0=a;1=b;
0;0;
array(2) {
  [0]=>
  int8(0)
  [1]=>
  int8(44)
}
string(2) "hi"
0;
2:ab
[][]
