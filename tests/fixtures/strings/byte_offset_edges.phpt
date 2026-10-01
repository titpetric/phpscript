name: byte offsets at and past the edges of a multi-byte subject
description: >
  Every offset-taking str* function over a subject whose bytes and characters
  differ: an offset landing inside a character, one at the exact end, one past
  it, and the negative forms of each. PHP clamps some of these and returns
  false for others, and which it does per function is the behaviour here.
---
<?php

$s = "ČedoŠ";

// substr: an offset inside a character, at the end, and past it.
var_dump(bin2hex(substr($s, 1)));
var_dump(substr($s, 7));
var_dump(substr($s, 8));
var_dump(substr($s, 99));
var_dump(bin2hex(substr($s, -1)));
var_dump(bin2hex(substr($s, -99)));
var_dump(substr($s, 0, 0));
var_dump(substr($s, 2, -2));
var_dump(substr($s, 4, -4));

// A negative $length stops that many bytes before the end, so -1 drops the
// last byte and a length that crosses the offset is the empty string.
var_dump(bin2hex(substr($s, 0, -1)));
var_dump(bin2hex(substr("abcdef", 0, -1)));
var_dump(substr("abcdef", 0, -6));
var_dump(substr("abcdef", 0, -9));
var_dump(substr("abcdef", -3, -1));
var_dump(substr("abcdef", -1, -3));
var_dump(substr("abcdef", 2, -2));
var_dump(bin2hex(substr($s, -3, -1)));
var_dump(substr_replace("abcdef", "X", 0, -1));
var_dump(substr_count("abcabc", "b", 0, -1));

// strpos: the offset is bytes, and one past the end is false rather than 0.
var_dump(strpos($s, "edo"));
var_dump(strpos($s, "edo", 2));
var_dump(strpos($s, "edo", 3));
var_dump(strpos($s, "", 7));
var_dump(strpos($s, "edo", -5));

// strrpos with a positive offset skips bytes, a negative one caps the match.
var_dump(strrpos("abcabc", "b"));
var_dump(strrpos("abcabc", "b", 2));
var_dump(strrpos("abcabc", "b", -2));
var_dump(strripos("aBcAbc", "B", -2));

// substr_count and substr_replace over the same window.
var_dump(substr_count($s, "o"));
var_dump(substr_count($s, "o", 5));
var_dump(bin2hex(substr_replace($s, "x", 1, 1)));
var_dump(bin2hex(substr_replace($s, "x", -1)));
var_dump(bin2hex(substr_replace($s, "x", 99)));

// str_pad counts bytes, so a multi-byte subject needs more of them.
var_dump(strlen(str_pad($s, 8, "-")));
var_dump(str_pad("ab", 7, "xy", STR_PAD_BOTH));
var_dump(str_pad("ab", 1, "-"));

// A string offset past the end, and str_split of a subject shorter than the
// chunk.
print_r(array_map("bin2hex", str_split($s, 4)));
print_r(str_split("ab", 9));
?>
---
string(12) "8c65646fc5a0"
string(0) ""
string(0) ""
string(0) ""
string(2) "a0"
string(14) "c48c65646fc5a0"
string(0) ""
string(3) "edo"
string(0) ""
string(12) "c48c65646fc5"
string(10) "6162636465"
string(0) ""
string(0) ""
string(2) "de"
string(0) ""
string(2) "cd"
string(4) "6fc5"
string(2) "Xf"
int(2)
int(2)
int(2)
bool(false)
int(7)
int(2)
int(4)
int(4)
int(4)
int(4)
int(1)
int(0)
string(14) "c47865646fc5a0"
string(14) "c48c65646fc578"
string(16) "c48c65646fc5a078"
int(8)
string(7) "xyabxyx"
string(2) "ab"
Array
(
    [0] => c48c6564
    [1] => 6fc5a0
)
Array
(
    [0] => ab
)
