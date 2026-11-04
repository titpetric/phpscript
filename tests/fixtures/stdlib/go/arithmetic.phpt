name: go typed arithmetic
runner:
  php: false
description: >
  A typed go value keeps its type through an operation and wraps at its width,
  which is go's arithmetic: int8(100) + int8(100) is int8(-56) where php's
  integers widen to a float. The increment, the compound operators, the unary
  operators and the shifts all compute at the width, and two different widths
  refuse the operation as go refuses it. php has no int8() to compare against,
  so no php run can answer for this output.
---
<?php

use Runtime\Go;

// Two values of one width answer that width and wrap in it.
var_dump(int8(100) + int8(100));
var_dump(int8(100) * int8(100));
var_dump(uint8(200) + uint8(100));

// A typed value against one of php's numbers answers the typed width, the way
// go reads an untyped constant, and the number converts to it.
var_dump(int8(100) + 1);
var_dump(2 * int8(50));
var_dump(uint8(0) - 1);
var_dump(uint64(0) - 1);

// Division truncates toward zero and keeps the type, where php's `/` answers a
// float for a remainder.
var_dump(int8(7) / int8(2));
var_dump(float32(1) / float32(3));

// The increment steps at the width, with no widening to a float at the end of
// it, and a compound operator reads the same arithmetic the binary one does.
$i = int8(127);
$i++;
var_dump($i);
$i--;
var_dump($i);
$u = uint8(0);
$u--;
var_dump($u);
$k = int8(1);
$k += 200;
var_dump($k);

// The unary operators wrap too: negating the smallest int8 answers itself, and
// inverting an unsigned zero answers the width's largest value where php's int
// answers -1.
var_dump(-int8(-128));
var_dump(~uint8(0));

// A shift computes at the width and the result narrows into it.
var_dump(int32(1) << 31);
var_dump(uint8(255) >> 4);
var_dump(int8(-8) >> 1);

// An unsigned value past PHP_INT_MAX prints the value go prints, and clips
// where php's int takes it.
$big = uint64(0) - 1;
echo $big, " ", sprintf("%d|%u", $big, $big), "\n";
var_dump((int) $big, intval($big), (float) $big);

// php's own comparisons read the number the value holds, and `===` reads the
// type: php's int is 64 bits wide and int8(44) is not it.
var_dump(int8(44) == 44, int8(44) === 44, int8(44) < 100);

// A cast reads the value as php reads a number, and a zero of any width is
// falsey.
var_dump((int) int8(44), (float) float32(0.5), (string) uint8(7));
var_dump((bool) int8(0), (bool) int8(1));

// Two different widths have no arithmetic between them: go refuses the pair,
// php words the refusal between operand types, and TypeError is the class it
// raises for it.
try {
    var_dump(int8(1) + int16(1));
} catch (TypeError $e) {
    echo $e->getMessage(), "\n";
}
---
int8(-56)
int8(16)
uint8(44)
int8(101)
int8(100)
uint8(255)
uint64(18446744073709551615)
int8(3)
float32(0.33333334)
int8(-128)
int8(127)
uint8(255)
int8(-55)
int8(-128)
uint8(255)
int32(-2147483648)
uint8(15)
int8(-4)
18446744073709551615 18446744073709551615|18446744073709551615
int(9223372036854775807)
int(9223372036854775807)
float(1.8446744073709552E+19)
bool(true)
bool(false)
bool(true)
int(44)
float(0.5)
string(1) "7"
bool(false)
bool(true)
Unsupported operand types: int8 + int16
