name: chr, ord
description: >
  chr reduces its argument modulo 256 and wraps negatives back into that range,
  and ord reports the first byte of a string, 0 for an empty one.
---
<?php
var_dump(chr(65));
var_dump(chr(97));
var_dump(chr(122));
var_dump(chr(321));
var_dump(strlen(chr(256)));
var_dump(ord(chr(256)));
var_dump(ord(chr(257)));
var_dump(strlen(chr(-1)));
var_dump(ord(chr(-1)));
var_dump(ord(chr(-256)));
var_dump(ord(chr(-257)));
var_dump(ord(chr(0)));
var_dump(ord(""));
var_dump(ord("ABC"));
var_dump(ord("a"));
var_dump(ord("\n"));
var_dump(ord("é"));
var_dump(chr(ord("A") + 1));
---
string(1) "A"
string(1) "a"
string(1) "z"
string(1) "A"
int(1)
int(0)
int(1)
int(1)
int(255)
int(0)
int(255)
int(0)
int(0)
int(65)
int(97)
int(10)
int(195)
string(1) "B"
