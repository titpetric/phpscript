name: mb_strlen, mb_substr, mb_strtolower, mb_strtoupper
description: >
  The mbstring functions count characters, and mb_substr accepts a negative
  offset and a negative length. The str* functions are aliases of the same
  implementations; the character-default divergence is pinned in
  mb_defaults.phpt.
---
<?php
$s = "héllo wörld";
var_dump(mb_strlen($s));
var_dump(mb_substr($s, 0, 3));
var_dump(mb_strlen(""));
var_dump(mb_strtoupper("héllo wörld"));
var_dump(mb_strtolower("HÉLLO WÖRLD"));
var_dump(mb_substr($s, -5));
var_dump(mb_substr($s, -5, 3));
var_dump(mb_substr($s, 2, -3));
var_dump(mb_substr($s, 0, -20));
var_dump(mb_substr($s, 20));
var_dump(mb_substr($s, 3, 0));
var_dump(mb_substr($s, -20, 4));
var_dump(mb_substr("", 0, 5));
var_dump(mb_strlen(mb_substr($s, 1, 4)));

// The "8bit" encoding selects byte units, the spelling for binary data.
var_dump(mb_strlen($s, "8bit"));
var_dump(mb_substr($s, 0, 3, "8bit"));
var_dump(mb_strpos($s, "wörld", 0, "8bit"));
print_r(mb_str_split($s, 4, "8bit"));
var_dump(mb_strlen(hash("sha256", "x", true), "8bit"));
?>

---
int(11)
string(4) "hél"
int(0)
string(13) "HÉLLO WÖRLD"
string(13) "héllo wörld"
string(6) "wörld"
string(4) "wör"
string(7) "llo wö"
string(0) ""
string(0) ""
string(0) ""
string(5) "héll"
string(0) ""
int(4)
int(13)
string(3) "hé"
int(7)
Array
(
    [0] => hél
    [1] => lo w
    [2] => örl
    [3] => d
)
int(32)
