name: divide float
description: Float division covers float, negative, mixed int operands and int-valued results.
---
<?php
echo 7.5 / 2.5, "\n";
echo 1.0 / 4, "\n";
echo -6.4 / 2, "\n";
echo 0.1 / 0.2, "\n";
$x = 5.0;
$x /= 2;
echo $x, "\n";
---
3
0.25
-3.2
0.5
2.5
