name: sprintf converts its arguments the way PHP renders a value
description: >
  Every specifier sprintf answers for, against the argument types a script
  passes. An argument is coerced before it is rendered, the way a binding
  declaring a string parameter coerces one (see string_argument_coercion.phpt),
  so %s of an int is its digits and %d of a numeric string is its value. Width,
  precision and padding count bytes, which is the unit the str* functions use.
  The exponent carries the digits it needs rather than being padded to two, an
  argument number selects an argument without advancing the implicit counter,
  and a specifier the format cannot name is a ValueError. The expected output
  is what php 8.5 prints for this source.
---
<?php

// Every argument type through %s: the conversion is PHP's string cast.
var_dump(sprintf("[%s]", "text"));
var_dump(sprintf("[%s]", 42));
var_dump(sprintf("[%s]", 1.5));
var_dump(sprintf("[%s]", 2.0));
var_dump(sprintf("[%s]", true));
var_dump(sprintf("[%s]", false));
var_dump(sprintf("[%s]", null));

// %d reads the leading numeric prefix of a string and truncates a float.
var_dump(sprintf("[%d]", "42"));
var_dump(sprintf("[%d]", "42abc"));
var_dump(sprintf("[%d]", "  7  "));
var_dump(sprintf("[%d]", ""));
var_dump(sprintf("[%d]", 3.9));
var_dump(sprintf("[%d]", true));
var_dump(sprintf("[%d]", null));
var_dump(sprintf("[%.2f]", 3));
var_dump(sprintf("[%.2f]", "3.5"));
var_dump(sprintf("[%.3f]", "1e3"));

// The unsigned conversions read the whole 64-bit pattern.
var_dump(sprintf("%u", 10));
var_dump(sprintf("%u", -1));
var_dump(sprintf("%b", 255));
var_dump(sprintf("%b", -1));
var_dump(sprintf("%o", 8));
var_dump(sprintf("%o", -1));
var_dump(sprintf("%x", 255));
var_dump(sprintf("%X", 255));
var_dump(sprintf("%x", -1));

// An exponent carries the digits it needs, not two.
var_dump(sprintf("%e", 1234.5));
var_dump(sprintf("%e", -1234.5));
var_dump(sprintf("%e", 0.0));
var_dump(sprintf("%E", 0.00012));
var_dump(sprintf("%e", 1.0e10));
var_dump(sprintf("%e", 1.0e100));
var_dump(sprintf("%.3e", 0.000123));
var_dump(sprintf("%.0e", 1500.0));
var_dump(sprintf("%g", 0.00001234));
var_dump(sprintf("%g", 123456789.0));
var_dump(sprintf("%G", 0.0000123));
var_dump(sprintf("%.3g", 1234.5));
var_dump(sprintf("%g", 0.0));
var_dump(sprintf("%g", 1.0));

// %f and %F default to six decimals; a negative zero loses its sign.
var_dump(sprintf("%f", 1.5));
var_dump(sprintf("%F", 1.5));
var_dump(sprintf("%.0f", 2.5));
var_dump(sprintf("%.20f", 0.1));
var_dump(sprintf("%f", -0.0));
var_dump(sprintf("%+f", 1.5));
var_dump(sprintf("%+e", 1.5));

// Width, precision and the padding flags.
var_dump(sprintf("%05.2f", 3.14159));
var_dump(sprintf("%+d %+d", 5, -5));
var_dump(sprintf("%05d", -42));
var_dump(sprintf("%+08d", -42));
var_dump(sprintf("%+08d", 42));
var_dump(sprintf("%08.2f", -4.2));
var_dump(sprintf("%-10s|", "abc"));
var_dump(sprintf("%10s|", "abc"));
var_dump(sprintf("%.2s", "abcdef"));
var_dump(sprintf("%.9s", "abc"));
var_dump(sprintf("[%8.3s]", "abcdef"));
var_dump(sprintf("[%-8.3s]", "abcdef"));
var_dump(sprintf("%08s", "ab"));
var_dump(sprintf("%-5d|", 42));
var_dump(sprintf("%-08d|", 42));
var_dump(sprintf("[%08x]", 255));
var_dump(sprintf("[%08b]", 5));
var_dump(sprintf("[%8.3d]", 42));
var_dump(sprintf("[% d]", 42));

// A padding character the format names, and the last one named winning.
var_dump(sprintf("%'x10s", "abc"));
var_dump(sprintf("%'010d", 42));
var_dump(sprintf("%'x8d", -42));
var_dump(sprintf("%-'x8d|", 42));
var_dump(sprintf("[%'a'b8d]", 42));
var_dump(sprintf("%'*8s", "ab"));

// Width and precision count bytes, as every str* function does.
var_dump(bin2hex(sprintf("%.2s", "\xc3\xa9x")));
var_dump(strlen(sprintf("%4s", "\xc3\xa9")));

// %c takes neither width nor precision.
var_dump(sprintf("%c", 65));
var_dump(sprintf("%c", "65"));
var_dump(sprintf("[%3c]", 65));
var_dump(bin2hex(sprintf("%c", 0)));
var_dump(bin2hex(sprintf("%c", 256)));
var_dump(bin2hex(sprintf("%c", -1)));

// An argument number selects an argument and leaves the implicit counter where
// it was, so a plain conversion after one reads the first argument again.
var_dump(sprintf('%2$s %1$s', "a", "b"));
var_dump(sprintf('%1$s%1$s', "x"));
var_dump(sprintf('%2$s %1$s %2$s', "a", "b"));
var_dump(sprintf('%1$s %s', "a", "b"));
var_dump(sprintf('%1$05d', 42));

// A literal percent, and arguments the format did not name.
var_dump(sprintf("100%%"));
var_dump(sprintf("[%s]", "a", "b"));

// What the format gets wrong.
try {
	sprintf("%v", 1);
} catch (ValueError $e) {
	echo get_class($e), ": ", $e->getMessage(), "\n";
}
try {
	sprintf("%s %s", "a");
} catch (ArgumentCountError $e) {
	echo get_class($e), ": ", $e->getMessage(), "\n";
}
try {
	sprintf('%3$s', "a");
} catch (ArgumentCountError $e) {
	echo get_class($e), ": ", $e->getMessage(), "\n";
}
try {
	sprintf('%0$s', "a");
} catch (ValueError $e) {
	echo get_class($e), ": ", $e->getMessage(), "\n";
}
try {
	sprintf("100%", "x");
} catch (ValueError $e) {
	echo get_class($e), ": ", $e->getMessage(), "\n";
}
?>
---
string(6) "[text]"
string(4) "[42]"
string(5) "[1.5]"
string(3) "[2]"
string(3) "[1]"
string(2) "[]"
string(2) "[]"
string(4) "[42]"
string(4) "[42]"
string(3) "[7]"
string(3) "[0]"
string(3) "[3]"
string(3) "[1]"
string(3) "[0]"
string(6) "[3.00]"
string(6) "[3.50]"
string(10) "[1000.000]"
string(2) "10"
string(20) "18446744073709551615"
string(8) "11111111"
string(64) "1111111111111111111111111111111111111111111111111111111111111111"
string(2) "10"
string(22) "1777777777777777777777"
string(2) "ff"
string(2) "FF"
string(16) "ffffffffffffffff"
string(11) "1.234500e+3"
string(12) "-1.234500e+3"
string(11) "0.000000e+0"
string(11) "1.200000E-4"
string(12) "1.000000e+10"
string(13) "1.000000e+100"
string(8) "1.230e-4"
string(4) "2e+3"
string(8) "1.234e-5"
string(10) "1.23457e+8"
string(7) "1.23E-5"
string(7) "1.23e+3"
string(1) "0"
string(1) "1"
string(8) "1.500000"
string(8) "1.500000"
string(1) "2"
string(22) "0.10000000000000000555"
string(8) "0.000000"
string(9) "+1.500000"
string(12) "+1.500000e+0"
string(5) "03.14"
string(5) "+5 -5"
string(5) "-0042"
string(8) "-0000042"
string(8) "+0000042"
string(8) "-0004.20"
string(11) "abc       |"
string(11) "       abc|"
string(2) "ab"
string(3) "abc"
string(10) "[     abc]"
string(10) "[abc     ]"
string(8) "000000ab"
string(6) "42   |"
string(9) "42      |"
string(10) "[000000ff]"
string(10) "[00000101]"
string(10) "[      42]"
string(4) "[42]"
string(10) "xxxxxxxabc"
string(10) "0000000042"
string(8) "xxxxx-42"
string(9) "42xxxxxx|"
string(10) "[bbbbbb42]"
string(8) "******ab"
string(4) "c3a9"
int(4)
string(1) "A"
string(1) "A"
string(3) "[A]"
string(2) "00"
string(2) "00"
string(2) "ff"
string(3) "b a"
string(2) "xx"
string(5) "b a b"
string(3) "a a"
string(5) "00042"
string(4) "100%"
string(3) "[a]"
ValueError: Unknown format specifier "v"
ArgumentCountError: 3 arguments are required, 2 given
ArgumentCountError: 4 arguments are required, 2 given
ValueError: Argument number specifier must be greater than zero and less than 2147483647
ValueError: Missing format specifier at end of string
