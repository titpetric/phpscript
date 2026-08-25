name: compound assignment
description: >
  Every compound assignment operator applies its operation and stores the
  result: the arithmetic set, the concatenation and the bitwise set. The
  operators are also right-hand-side aware, so `$a *= 1 + 1` multiplies by the
  sum rather than by the first operand.
---
<?php

$a = 5;
$a += 3;
echo "add=", $a, "\n";

$a -= 2;
echo "sub=", $a, "\n";

$a *= 3;
echo "mul=", $a, "\n";

$a /= 6;
echo "div=", $a, "\n";

$a = 17;
$a %= 5;
echo "mod=", $a, "\n";

$a = 2;
$a **= 5;
echo "pow=", $a, "\n";

$s = "a";
$s .= "b";
echo "concat=", $s, "\n";

$b = 12;
$b &= 10;
echo "and=", $b, "\n";

$b |= 5;
echo "or=", $b, "\n";

$b ^= 3;
echo "xor=", $b, "\n";

$b <<= 2;
echo "shl=", $b, "\n";

$b >>= 1;
echo "shr=", $b, "\n";

$c = 4;
$c *= 1 + 1;
echo "rhs=", $c, "\n";

$d = 7;
$d /= 2;
echo "float=", $d, "\n";
---
add=8
sub=6
mul=18
div=3
mod=2
pow=32
concat=ab
and=8
or=13
xor=14
shl=56
shr=28
rhs=8
float=3.5
