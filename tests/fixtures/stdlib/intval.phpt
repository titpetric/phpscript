name: intval matches php
description: >
  intval without a base, or with base 10, is the (int) cast: the leading
  numeric prefix of a string, truncation toward zero for a float, saturation
  at the int bounds, and truthiness for an array. Any other base applies to
  a string only and reads it like C strtol: whitespace, one sign, a 0x or 0b
  prefix for bases 16, 2 and 0, a leading 0 as octal for base 0, digits until
  the first invalid one, saturating on overflow. A base outside 0 and 2-36
  yields 0 rather than a ValueError, and the 0o prefix is not recognised.
  Verified against php 8.5.
---
<?php

// The (int) cast path: no base, or the default base 10.
var_dump(intval("42"));
var_dump(intval(4.7));
var_dump(intval(-4.7));
var_dump(intval(true));
var_dump(intval(false));
var_dump(intval(null));
var_dump(intval("  12abc"));
var_dump(intval("1e3"));
var_dump(intval("-1.9"));
var_dump(intval("0x1A"));
var_dump(intval("012"));
var_dump(intval("hello"));
var_dump(intval("9223372036854775808"));
var_dump(intval("-9223372036854775809"));
var_dump(intval([]));
var_dump(intval([1, 2]));

// A base other than 10 applies to a string only.
var_dump(intval("0x1A", 16));
var_dump(intval("1A", 16));
var_dump(intval("-0x1A", 16));
var_dump(intval("  +0xff", 16));
var_dump(intval("012", 8));
var_dump(intval("0o12", 8));
var_dump(intval("0b101", 2));
var_dump(intval("101", 2));
var_dump(intval("z", 36));
var_dump(intval("zz", 36));
var_dump(intval("1e3", 10));
var_dump(intval("1e3", 16));
var_dump(intval("42abc", 10));

// Base 0 detects the base from the prefix.
var_dump(intval("0x1A", 0));
var_dump(intval("0b101", 0));
var_dump(intval("012", 0));
var_dump(intval("42", 0));
var_dump(intval("018", 0));
var_dump(intval("", 0));

// Overflow saturates at the int bounds; a non-string ignores the base;
// a base outside 0 and 2-36 yields 0.
var_dump(intval("ffffffffffffffffff", 16));
var_dump(intval("-ffffffffffffffffff", 16));
var_dump(intval(12.9, 16));
var_dump(intval("12", 1));
var_dump(intval("12", 37));
var_dump(intval(12, 37));
---
int(42)
int(4)
int(-4)
int(1)
int(0)
int(0)
int(12)
int(1000)
int(-1)
int(0)
int(12)
int(0)
int(9223372036854775807)
int(-9223372036854775808)
int(0)
int(1)
int(26)
int(26)
int(-26)
int(255)
int(10)
int(0)
int(5)
int(5)
int(35)
int(1295)
int(1000)
int(483)
int(42)
int(26)
int(5)
int(10)
int(42)
int(1)
int(0)
int(9223372036854775807)
int(-9223372036854775808)
int(12)
int(0)
int(0)
int(12)
