name: float format
description: >
  Floats echo with precision=14 and PHP's exponent style: mantissa with a
  decimal point, no leading zero in the exponent, and negative zero as -0.
---
<?php
echo 1 / 3, "\n";
echo 1e20, "\n";
echo 1e-7, "\n";
echo 0.00001, "\n";
echo 0.0001, "\n";
echo 100000000000000.5, "\n";
echo 123456789.123456789, "\n";
echo -0.0, "\n";
echo 1.5e20, "\n";
---
0.33333333333333
1.0E+20
1.0E-7
1.0E-5
0.0001
1.0E+14
123456789.12346
-0
1.5E+20
