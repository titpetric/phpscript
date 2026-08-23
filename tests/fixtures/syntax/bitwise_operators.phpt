name: bitwise operators
description: >
  Covers PHP bitwise syntax: the binary operators & | ^ << >>, the unary
  complement ~, their precedence against each other and against comparison,
  concatenation and addition, the int coercion of their operands, the bytewise
  string forms, and the compound assignments &= |= ^= <<= >>=.
---
<?php

// Binary operators on integers.
echo 1 | 2, ";", 6 & 3, ";", 5 ^ 3, ";", 1 << 3, ";", 6 >> 1, "\n";
echo 0x0F & 0xF0, ";", 0x0F | 0xF0, ";", 0x0F ^ 0xFF, "\n";

// Unary complement: ~n is -(n+1).
echo ~5, ";", ~-6, ";", ~ ~5, ";", ~0, "\n";

// Precedence. `|` is looser than `^`, `^` looser than `&`, all three looser
// than a comparison; `<<` and `>>` bind tighter than `.` and looser than `+`.
echo 1 | 2 == 2, ";", 7 & 3 == 3, ";", 1 < 2 & 3, "\n";
echo 1 ^ 2 & 3, ";", 2 | 4 ^ 6, ";", 6 & 3 | 1 ^ 2, "\n";
echo 1 << 2 + 3, ";", "a" . 1 << 2, ";", 2 ** 3 ^ 1, "\n";
echo 1 << 2 << 3, ";", 16 >> 1 >> 2, ";", -(1 << 2), "\n";
echo (1 | 2) == 2 ? "y" : "n", ";", (1 << 2) + 3, "\n";

// Operands are cast to int.
echo true | 0, ";", 1.9 & 3, ";", null | 5, ";", "12" & 3, ";", "3" & 1, "\n";

// Shifts are int64 operations: an over-wide count is 0, or -1 for a negative
// right operand, and the left shift wraps.
echo -8 >> 1, ";", -1 >> 63, ";", -1 >> 64, ";", 5 >> 70, ";", 1 << 64, "\n";
echo PHP_INT_MAX << 1, ";", ~PHP_INT_MAX, "\n";

// A negative shift count has no answer; PHP raises ArithmeticError.
try {
    echo 1 << -1;
} catch (ArithmeticError $e) {
    echo get_class($e), ":", $e->getMessage(), "\n";
}

// & | ^ between two strings work on bytes and yield a string. `&` and `^` stop
// at the shorter operand, `|` keeps the longer one.
echo "a" | "b", ";", "abc" & "ab", ";", "abc" | "ab", ";", strlen("abc" ^ "ab"), "\n";
echo ("10" ^ "3") === "\x02" ? "y" : "n", ";", (~ ~"abc") === "abc" ? "y" : "n", ";", strlen(~"abc"), ";", "Hello" ^ "    ", "\n";

// Compound assignment.
$x = 6;
$x &= 3;
echo $x, ";";
$x |= 4;
echo $x, ";";
$x ^= 3;
echo $x, ";";
$x <<= 4;
echo $x, ";";
$x >>= 2;
echo $x, "\n";

$s = "abc";
$s &= "ab";
echo $s, "\n";

$flags = array("mask" => 6);
$flags["mask"] &= 3;
$flags["mask"] |= 8;
echo $flags["mask"], "\n";

class Bits
{
    public $mask = 12;
}
$bits = new Bits();
$bits->mask &= 10;
$bits->mask <<= 1;
echo $bits->mask, "\n";

try {
    $shift = 1;
    $shift >>= -2;
    echo $shift;
} catch (ArithmeticError $e) {
    echo get_class($e), ":", $e->getMessage(), "\n";
}

// The combination that constant flags are written with.
$mask = 0;
foreach (array(1, 2, 4, 8) as $bit) {
    $mask |= $bit;
}
echo $mask, ";", $mask & ~4, ";", ($mask & 8) != 0 ? "set" : "clear", "\n";
---
3;2;6;8;3
0;255;240
-6;5;5;-1
1;1;1
3;2;3
32;a4;9
32;2;-4
n;7
1;1;5;0;1
-4;-1;-1;0;0
-2;-9223372036854775808
ArithmeticError:Bit shift by negative number
c;ab;abc;2
y;y;3;hELL
2;6;5;80;20
ab
10
16
ArithmeticError:Bit shift by negative number
15;11;set
