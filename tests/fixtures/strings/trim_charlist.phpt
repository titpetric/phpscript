name: the trim family reads its character list as a byte set with ranges
description: >
  trim(), ltrim() and rtrim() take a list of characters rather than a string to
  strip, and "a..z" in that list stands for every character between the two.
  The list is a set of bytes, which is the unit the rest of the str* functions
  count in, so a list naming one byte of a multi-byte character strips that
  byte. A range missing an end, or one whose end sorts below its start, is read
  as the literal characters it is written with; php warns on each of those and
  there are no warnings here, so only the reading is pinned. The expected
  output is what php 8.5 prints for this source.
---
<?php

// A range covers every byte between its ends, both included.
var_dump(trim("abcHELLOcba", "a..c"));
var_dump(trim("12abc34", "0..9"));
var_dump(ltrim("123hello", "0..9"));
var_dump(rtrim("hello123", "0..9"));
var_dump(trim("aXa", "a..a"));
var_dump(trim("abc", "a..cx"));
var_dump(trim("0a9z", "0..9z"));
var_dump(trim("za0b9", "z0..9"));

// A dot outside a range is a character like any other.
var_dump(trim("..a..", "."));
var_dump(trim("a..b", "a..b"));
var_dump(trim("x.y", ".."));
var_dump(trim("xa-zAxBz", "a-z"));

// A malformed range is the characters it is written with.
var_dump(trim("x", "z..a"));
var_dump(trim("x.y", "..y"));
var_dump(trim("x.y", "x.."));
var_dump(trim("a.b", "a..."));

// The list is bytes, so one byte of a character is one member of the set.
var_dump(bin2hex(trim("\xc3\xa9x\xc3\xa9", "\xc3")));
var_dump(bin2hex(trim("\xfe\xffx\xff", "\xfe..\xff")));

// What a call that names no list strips, and the plain cases.
var_dump(trim("  \t\r\n hi \0\x0B"));
var_dump(trim("xxhelloxx", "x"));
var_dump(ltrim("  \t hi"));
var_dump(rtrim("hello...", "."));
var_dump(trim("  hi  ", ""));
var_dump(trim(""));
var_dump(trim("aaa", "a"));
?>
---
string(5) "HELLO"
string(3) "abc"
string(5) "hello"
string(5) "hello"
string(1) "X"
string(0) ""
string(1) "a"
string(3) "a0b"
string(1) "a"
string(2) ".."
string(3) "x.y"
string(7) "xa-zAxB"
string(1) "x"
string(1) "x"
string(1) "y"
string(1) "b"
string(8) "a978c3a9"
string(2) "78"
string(2) "hi"
string(5) "hello"
string(2) "hi"
string(5) "hello"
string(6) "  hi  "
string(0) ""
string(0) ""
