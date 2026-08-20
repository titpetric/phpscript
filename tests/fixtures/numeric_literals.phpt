name: numeric literals
description: >
  Covers PHP's integer and float literal spellings: decimal, hexadecimal,
  binary, octal in both the 0o and the legacy leading-zero form, the underscore
  digit separator, and floats with a fractional part or an exponent. The octal
  form is what a chmod() argument is written in.
---
<?php

echo 42 . ";";
echo 0x1F . ";";
echo 0XdeadBEEF . ";";
echo 0b1010 . ";";
echo 0o17 . ";";
echo 017 . ";";
echo 0 . ";";
echo 1_000_000 . ";";
echo 0644 . ";";

echo 1.5 . ";";
echo 1e3 . ";";
echo 1E-3 . ";";
echo 1.5e2 . ";";
echo 1_000.5 . ";";

$mode = 0755;
echo $mode . ";";
echo 0x10 + 0b1 . ";";
---
42;31;3735928559;10;15;15;0;1000000;420;1.5;1000;0.001;150;1000.5;493;17;
