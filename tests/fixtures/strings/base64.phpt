name: base64_encode, base64_decode
description: >
  Binary data round-trips through base64, and base64_decode skips characters
  outside the alphabet unless $strict is set, where it returns false instead.
---
<?php
$bin = "";
for ($i = 0; $i < 16; $i++) {
    $bin .= chr($i * 17);
}
$enc = base64_encode($bin);
echo $enc, "\n";
var_dump(base64_decode($enc) === $bin);
echo base64_encode("hello"), "\n";
echo base64_encode("hi"), "\n";
var_dump(base64_decode("aGVsbG8="));
var_dump(base64_decode("aGVs!!bG8="));
var_dump(base64_decode("aGVs!!bG8=", true));
var_dump(base64_decode("aGVsbG8", true));
var_dump(base64_decode("aGVsbG8 =", true));
var_dump(base64_decode("a", true));
var_dump(base64_decode("aGVsbG8==", true));
var_dump(base64_decode(""));
---
ABEiM0RVZneImaq7zN3u/w==
bool(true)
aGVsbG8=
aGk=
string(5) "hello"
string(5) "hello"
bool(false)
string(5) "hello"
string(5) "hello"
bool(false)
bool(false)
string(0) ""
