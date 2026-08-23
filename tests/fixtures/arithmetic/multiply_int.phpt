name: multiply int
description: Integer multiplication covers positive, negative and zero operands.
---
<?php
echo 6 * 7, "\n";
echo -4 * 5, "\n";
echo -3 * -9, "\n";
echo 0 * 12345, "\n";
echo 1000000 * 1000000, "\n";
$a = 12;
$b = 12;
echo $a * $b, "\n";
$a *= 2;
echo $a, "\n";
---
42
-20
27
0
1000000000000
144
24
