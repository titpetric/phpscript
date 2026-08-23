name: intdiv and fdiv
description: >
  intdiv truncates toward zero; fdiv is IEEE-754 division, so dividing by
  zero yields INF, -INF or NAN instead of an error.
---
<?php
echo intdiv(7, 2), "\n";
echo intdiv(-7, 2), "\n";
echo intdiv(10, 5), "\n";
echo fdiv(5.0, 2.0), "\n";
echo fdiv(1, 0), "\n";
echo fdiv(-1, 0), "\n";
echo fdiv(0, 0) != fdiv(0, 0) ? "nan" : "num", "\n";
---
3
-3
2
2.5
INF
-INF
nan
