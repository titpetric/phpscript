name: subtract float
description: Float subtraction covers precision, mixed int operands and int-valued results.
---
<?php
echo 2.5 - 1.25, "\n";
echo 0.3 - 0.1, "\n";
echo 1 - 0.5, "\n";
echo 5.5 - 5.5, "\n";
$x = 10.5;
$x -= 3;
echo $x, "\n";
---
1.25
0.2
0.5
0
7.5
