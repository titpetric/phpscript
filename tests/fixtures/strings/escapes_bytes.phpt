name: a hex escape names one byte, whatever its value
description: >
  "\xff" is one byte, not the codepoint U+00FF. The distinction only shows above
  7f, where UTF-8 spells a codepoint with two bytes: a string that travelled
  through source text as an escape would come back a byte longer. Covers the
  boundary either side of 7f and a byte written into the middle of a string.
---
<?php

echo strlen("a\xffb"), " ", bin2hex("a\xffb"), "\n";
echo strlen("\x7f"), " ", bin2hex("\x7f"), "\n";
echo strlen("\x80"), " ", bin2hex("\x80"), "\n";
echo strlen("\x00"), " ", bin2hex("\x00"), "\n";

// The same bytes built with chr(), which never travels as an escape, so the
// two spellings have to agree.
echo bin2hex("a" . chr(255) . "b"), "\n";
var_dump("a\xffb" === "a" . chr(255) . "b");

// An octal escape is a byte too.
echo strlen("\377"), " ", bin2hex("\377"), "\n";
?>
---
3 61ff62
1 7f
1 80
1 00
61ff62
bool(true)
1 ff
