name: str* functions and string offsets count characters
description: >
  The str* functions and $s[$i] count characters, not bytes, and the mb_*
  names are aliases of the same implementations: strlen("héllo wörld") is
  11, substr slices whole characters, the strpos family speaks character
  offsets, strrev and ucfirst keep multi-byte letters intact, and a string
  walked with strlen and $s[$i] rebuilds identically. PHP's own str* count
  bytes (its strlen answers 13 and its substr cuts é in half), which is why
  the php runner is opted out; every character value below equals what
  PHP's mb_* functions answer.
runner:
  php: false
---
<?php

$s = "héllo wörld";

// Lengths and slices count characters, and agree with the mb_ spelling.
var_dump(strlen($s));
var_dump(strlen($s) === mb_strlen($s));
var_dump(substr($s, 0, 3));
var_dump(substr($s, 0, 3) === mb_substr($s, 0, 3));
var_dump(substr($s, -5));

// The strpos family speaks character offsets.
var_dump(strpos($s, "wörld"));
var_dump(strpos($s, "wörld") === mb_strpos($s, "wörld"));
var_dump(stripos($s, "WÖRLD"));
var_dump(strrpos($s, "l"));
var_dump(substr($s, strpos($s, "wörld")));

// Multi-byte letters survive reversal, splitting and case changes.
echo strrev("Čedo"), "\n";
print_r(str_split("Čedo", 2));
var_dump(ucfirst("ábc"));
var_dump(lcfirst("Ábc"));
var_dump(str_pad("Če", 4, "š"));
var_dump(substr_count("šašav", "š", 1));
var_dump(substr_replace("Čedo", "u", 1, 2));

// A string offset is one character, negative offsets count from the end.
$name = "Čedo";
var_dump($name[0]);
var_dump($name[-1]);
var_dump($name[7]);

// Walking with strlen and $s[$i] rebuilds the string (github issue 98).
$copy = "";
for ($i = 0; $i < strlen($name); $i++) {
	$copy .= $name[$i];
}
var_dump($copy === $name);
---
int(11)
bool(true)
string(4) "hél"
bool(true)
string(6) "wörld"
int(6)
bool(true)
int(6)
int(9)
string(6) "wörld"
odeČ
Array
(
    [0] => Če
    [1] => do
)
string(4) "Ábc"
string(4) "ábc"
string(7) "Češš"
int(1)
string(4) "Čuo"
string(2) "Č"
string(1) "o"
string(0) ""
bool(true)
