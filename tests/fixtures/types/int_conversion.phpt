name: a float outside the integer range wraps, as php converts one
description: >
  Go's conversion of an out-of-range float to int64 is implementation defined and
  saturates on amd64, so every value too large became the minimum integer. PHP
  wraps modulo 2^64 into the signed range, which is what C's (int64_t) cast does
  and what zend_dval_to_lval implements.

  The same conversion feeds the modulo and bitwise operators, so those disagreed
  too. An array converts to 1 when it holds anything and 0 when it does not,
  which phpscript answered as 0 either way.

  NAN and INF are not registered constants here, so the NaN case arrives through
  sqrt(-1); the infinities are covered by TestToInt64 in internal/phpval.
---
<?php
$vals = [1.0e20, -1.0e20, 1.0e19, 9.3e18, 1.8e19, 2.0**63, 2.0**64, 2.0**64+2.0**63, 1.9, -1.9, 0.0, 9.0e18];
foreach ($vals as $v) { echo var_export($v, true), " => ", (int)$v, "\n"; }
echo "mod: ", 1.0e20 % 7, "\n";
echo "shl: ", 1.0e20 << 1, "\n";
echo "and: ", 1.0e20 & 255, "\n";
echo "arr: [1,2]=", (int)[1,2], " []=", (int)[], "\n";
echo "nan: ", (int)sqrt(-1), "\n";
---
1.0E+20 => 7766279631452241920
-1.0E+20 => -7766279631452241920
1.0E+19 => -8446744073709551616
9.3E+18 => -9146744073709551616
1.8E+19 => -446744073709551616
9.223372036854776E+18 => -9223372036854775808
1.8446744073709552E+19 => 0
2.7670116110564327E+19 => -9223372036854775808
1.9 => 1
-1.9 => -1
0.0 => 0
9.0E+18 => 9000000000000000000
mod: 6
shl: -2914184810805067776
and: 0
arr: [1,2]=1 []=0
nan: 0
