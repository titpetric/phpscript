name: number_format
description: >
  number_format rounds half away from zero, groups the integer part every
  three digits and takes both separators as arguments.
---
<?php
echo number_format(1234567.891, 2), "\n";
echo number_format(1234567.891), "\n";
echo number_format(2.5), "\n";
echo number_format(-0.5), "\n";
echo number_format(0.5), "\n";
echo number_format(-0.4), "\n";
echo number_format(1234.5678, 2, ",", "."), "\n";
echo number_format(1234567.891, 2, ".", ""), "\n";
echo number_format(123, 2), "\n";
echo number_format(-1234.5), "\n";
var_dump(number_format(1234567.891, 2));
---
1,234,567.89
1,234,568
3
-1
1
0
1.234,57
1234567.89
123.00
-1,235
string(12) "1,234,567.89"
