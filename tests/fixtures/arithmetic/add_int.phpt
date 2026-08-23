name: add int
description: Integer addition covers positive, negative and zero operands.
---
<?php
echo 1 + 2, "\n";
echo -4 + 9, "\n";
echo 0 + 0, "\n";
echo 2000000000 + 2000000000, "\n";
$a = 40;
$a += 2;
echo $a, "\n";
---
3
5
0
4000000000
42
