name: multiply float
description: Float multiplication covers float, negative, mixed int operands and int-valued results.
---
<?php
echo 1.5 * 2.5, "\n";
echo 0.1 * 0.2, "\n";
echo -2.5 * 4.0, "\n";
echo 3 * 0.5, "\n";
echo 2 * 2.5, "\n";
$x = 1.25;
$x *= 4;
echo $x, "\n";
echo 2.0 * 2, "\n";
echo 0.5 * 8, "\n";
---
3.75
0.02
-10
1.5
5
5
4
4
