name: go conversions
runner:
  php: false
description: >
  A go builtin type converts a value to its own width, and the name is a class
  the file can allocate through. The parser folds a conversion of a literal and
  the binding converts a value, and both read one table, so the two answers
  agree. php has no int64() to compare against, so no php run can answer for
  this output.
---
<?php

use Runtime\Go;

// A narrow type keeps the low bits of the value, as go's conversion does. The
// carrier is php's int, which is the 64 bit one.
var_dump(int8(300));
var_dump(uint8(-1));
var_dump(int32(2147483648));
var_dump(float64(3));
var_dump(float32(0.5));

// A value php has to read first reaches the binding, which reduces it through
// php's own semantics and applies the same conversion to the result.
var_dump(int64("12"));
var_dump(string(5));
var_dump(bool("0"));

// Nothing to convert is the type's zero value, which is what go's conversion
// of a zero value reads back.
var_dump(int64());

// A literal folds while the file parses and a variable converts at run time,
// to the same value.
$n = 300;
var_dump(int8(300) === int8($n));

// float32 is the width that loses precision: the value reads back as the
// float64 nearest the float32, php carrying one float type.
var_dump(float32(0.1) === 0.1);

// A go type is in the class table, and new allocates through the conversion.
var_dump(class_exists('Runtime\Go\int64'));
var_dump(new Go\int64("7"));
---
int(44)
int(255)
int(-2147483648)
float(3)
float(0.5)
int(12)
string(1) "5"
bool(false)
int(0)
bool(true)
bool(false)
bool(true)
int(7)
