name: the str* functions count bytes and the mb_* functions count characters
description: >
  PHP's split between the two families: strlen, substr, the strpos family,
  str_split, str_pad, strrev, the case functions and $s[$i] count bytes, and
  the mb_ spelling of each counts characters. The byte offset strpos reports is
  the one substr takes, which is what makes PREG_OFFSET_CAPTURE usable. A byte
  cut landing inside a character leaves invalid UTF-8, so those results are
  shown as bin2hex rather than as a replacement glyph.
---
<?php

$s = "héllo wörld";

var_dump(strlen($s));
var_dump(mb_strlen($s));
var_dump(substr($s, 0, 3));
var_dump(mb_substr($s, 0, 3));
var_dump(bin2hex(substr($s, -5)));
var_dump(mb_substr($s, -5));

// The strpos family reports byte offsets, mb_strpos character offsets.
var_dump(strpos($s, "wörld"));
var_dump(mb_strpos($s, "wörld"));
var_dump(stripos($s, "WöRLD"));
var_dump(strrpos($s, "l"));
var_dump(mb_strrpos($s, "l"));

// The offset strpos reports is the one substr takes.
var_dump(substr($s, strpos($s, "wörld")));
var_dump(mb_substr($s, mb_strpos($s, "wörld")));

// A case fold is ASCII-only, so a non-ASCII letter is left alone.
var_dump(strtoupper("ábc"));
var_dump(mb_strtoupper("ábc"));
var_dump(ucfirst("ábc"));
var_dump(mb_ucfirst("ábc"));
var_dump(lcfirst("Ábc"));
var_dump(mb_lcfirst("Ábc"));

// Splitting, padding, replacement and reversal count bytes.
print_r(array_map("bin2hex", str_split("Čedo", 2)));
print_r(mb_str_split("Čedo", 2));
var_dump(str_pad("Če", 6, "-"));
var_dump(mb_str_pad("Če", 6, "-"));
var_dump(bin2hex(substr_replace("Čedo", "u", 1, 2)));
var_dump(bin2hex(strrev("Čedo")));
var_dump(substr_count("šašav", "š", 1));
var_dump(mb_substr_count("šašav", "š"));

// A string offset is one byte, negative offsets count from the end.
$name = "Čedo";
var_dump(bin2hex($name[0]));
var_dump($name[-1]);
var_dump(mb_substr($name, 0, 1));

// Walking with strlen and $s[$i] rebuilds the string byte by byte.
$copy = "";
for ($i = 0; $i < strlen($name); $i++) {
	$copy .= $name[$i];
}
var_dump($copy === $name);
?>
---
int(13)
int(11)
string(3) "hé"
string(4) "hél"
string(10) "c3b6726c64"
string(6) "wörld"
int(7)
int(6)
int(7)
int(11)
int(9)
string(6) "wörld"
string(6) "wörld"
string(4) "áBC"
string(4) "ÁBC"
string(4) "ábc"
string(4) "Ábc"
string(4) "Ábc"
string(4) "ábc"
Array
(
    [0] => c48c
    [1] => 6564
    [2] => 6f
)
Array
(
    [0] => Če
    [1] => do
)
string(6) "Če---"
string(7) "Če----"
string(8) "c475646f"
string(10) "6f64658cc4"
int(1)
int(2)
string(2) "c4"
string(1) "o"
string(2) "Č"
bool(true)
