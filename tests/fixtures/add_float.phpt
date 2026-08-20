name: add float
description: Float addition covers precision, negative, mixed int operands and int-valued results.
---
<?php
echo 0.1 + 0.2, "\n";
echo 1.5 + 2.25, "\n";
echo -1.5 + 0.5, "\n";
echo 2 + 0.5, "\n";
$x = 0.75;
$x += 0.25;
echo $x, "\n";
---
0.3
3.75
-1
2.5
1
