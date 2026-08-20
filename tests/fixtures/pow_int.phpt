name: pow int
description: >
  Integer exponentiation covers precedence over unary minus, right
  associativity, zero and negative exponents, overflow to float and **=.
---
<?php
echo 2 ** 10, "\n";
echo -2 ** 2, "\n";
echo (-2) ** 3, "\n";
echo 2 ** 3 ** 2, "\n";
echo 2 ** 0, "\n";
echo 2 ** -1, "\n";
echo 2 ** 63, "\n";
$a = 3;
$a **= 4;
echo $a, "\n";
---
1024
-4
-8
512
1
0.5
9.2233720368548E+18
81
