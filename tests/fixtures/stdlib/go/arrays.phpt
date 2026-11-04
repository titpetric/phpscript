name: go arrays
runner:
  php: false
description: >
  go's builtins read php's array as well as go's slice: append writes through
  the handle a script holds, len and delete and clear take one, and a null is
  the empty slice it is in go. php has no append() to compare against, so no
  php run can answer for this output.
---
<?php

use Runtime\Go;

// append writes through the array handle, and the handle it returns is the one
// it was given: a php array grows in place, where go's append returns a slice
// that aliases its argument only while the capacity holds.
$a = [1, 2];
append($a, 3);
echo count($a), ":", implode(",", $a), "\n";

$b = append($a, 4);
echo count($a), ":", count($b), "\n";

// A null has no element type to allocate from, so a php array is what it grows
// into.
$c = append(null, "x", "y");
echo count($c), ":", implode(",", $c), "\n";

// len is go's: a string's bytes, an array's entries, and nothing for a null.
echo len("héllo"), ",", len([1, 2, 3]), ",", len(null), "\n";

// delete takes a key, which is a php array's as much as a go map's, and clear
// empties what it is given.
$m = ["x" => 1, "y" => 2];
delete($m, "x");
echo implode(",", array_keys($m)), "\n";

clear($m);
echo count($m), "\n";

// The import binds min and max: a bare one is go's, reading values where
// php's reads an array, and the leading separator reaches php's.
echo min(3, 1, 2), Go\max(3, 1, 2), \min([3, 1, 2]), "\n";

// println writes to the output a script echoes to, spaced, with a newline.
println("a", 1);
---
3:1,2,3
4:4
2:x,y
6,3,0
y
0
131
a 1
