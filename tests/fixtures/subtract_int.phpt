name: subtract int
description: Integer subtraction covers positive, negative and zero operands.
---
<?php
echo 9 - 4, "\n";
echo 4 - 9, "\n";
echo -3 - -7, "\n";
echo 0 - 12345, "\n";
$a = 50;
$a -= 8;
echo $a, "\n";
---
5
-5
4
-12345
42
