name: go builtins read a slice and a map
runner:
  php: false
description: >
  The import binds go's builtins and go's composite literals: `[...]` is a
  []any and `{...}` a map[string]any. append answers the slice it grew, since
  a two-element literal has no capacity to append into, len reports a slice's
  length, and delete removes a map key. php writes no braced map, so only
  phpscript defines this output.
---
<?php

use Runtime\Go;

// append answers the slice it grew. The literal is full, so appending
// reallocates and only the result holds the third element.
$a = [1, 2];
$b = append($a, 3);
echo count($a), ":", count($b), ":", len($b), "\n";

// A null has no element type to allocate from, so a php array is what it grows
// into.
$c = append(null, "x", "y");
echo count($c), ":", implode(",", $c), "\n";

// len is go's: a string's bytes, a slice's entries, and nothing for a null.
echo len("héllo"), ",", len([1, 2, 3]), ",", len(null), "\n";

// delete takes a map key, and clear empties the map.
$m = {"x": 1, "y": 2};
delete($m, "x");
echo len($m), ":", $m["y"], "\n";

clear($m);
echo len($m), "\n";

// The import binds min and max: a bare one is go's, reading values where
// php's reads an array, and the leading separator reaches php's.
echo min(3, 1, 2), Go\max(3, 1, 2), \min([3, 1, 2]), "\n";

// println writes to the output a script echoes to, spaced, with a newline.
println("a", 1);
---
2:3:3
2:x,y
6,3,0
1:2
0
131
a 1
