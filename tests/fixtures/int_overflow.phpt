name: int overflow
description: >
  Integer addition, subtraction and multiplication that overflow 64 bits
  become float, as in PHP, instead of wrapping around.
---
<?php
echo 9223372036854775807 + 1, "\n";
echo 9223372036854775807 * 2, "\n";
echo -9223372036854775807 - 2, "\n";
echo 9223372036854775807 + 9223372036854775807, "\n";
echo 4611686018427387904 * 2, "\n";
---
9.2233720368548E+18
1.844674407371E+19
-9.2233720368548E+18
1.844674407371E+19
9.2233720368548E+18
