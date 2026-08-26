name: random_bytes and random_int produce usable entropy
description: >
  Randomness cannot be asserted by value, so this pins length, the bin2hex
  round trip and inclusive bounds instead. Runs under php unchanged.
---
<?php

$b = random_bytes(16);
echo strlen($b), "\n";
echo strlen(bin2hex($b)), "\n";
var_dump(hex2bin(bin2hex($b)) === $b);

$n = random_int(1, 6);
var_dump($n >= 1 && $n <= 6);
var_dump(random_int(5, 5));
$m = random_int(-3, 3);
var_dump($m >= -3 && $m <= 3);

echo bin2hex("Hi\x00\x7f"), "\n";
var_dump(hex2bin("486921") === "Hi!");
var_dump(hex2bin("486921") === hex2bin("486921"));
---
16
32
bool(true)
bool(true)
int(5)
bool(true)
4869007f
bool(true)
bool(true)
