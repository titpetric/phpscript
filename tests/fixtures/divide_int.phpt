name: divide int
description: Integer division covers even, uneven, negative operands and float results.
---
<?php
echo 6 / 3, "\n";
echo 7 / 2, "\n";
echo -9 / 3, "\n";
echo 10 / 4, "\n";
echo 1 / 8, "\n";
$a = 100;
$a /= 5;
echo $a, "\n";
---
2
3.5
-3
2.5
0.125
20
