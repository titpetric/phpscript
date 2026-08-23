name: scalar casts
description: >
  The (int), (float), (string), (bool) and (array) casts, on the inputs a
  request handler meets: numeric strings from query parameters, floats
  truncated to page numbers, and scalars wrapped for a uniform foreach. A cast
  is a *model.Cast node, which the flatstack compiler does not lower yet; this
  fixture holds the interpreter fallback and, later, the native path to the
  same output. dbadmin casts $_GET["page"] and $_PATH["rowid"] this way.
---
<?php

$page = (int)"3";
echo $page + 1, "\n";

echo (int)"12abc", "\n";
echo (int)"", "\n";
echo (int)3.9, "\n";
echo (int)-3.9, "\n";
echo (int)true, ":", (int)false, "\n";
echo is_int((int)"7") ? "int" : "not-int", "\n";

echo (float)"2.5", "\n";
echo (float)7, "\n";

echo (string)42, "\n";
echo (string)3.5, "\n";
echo is_string((string)42) ? "string" : "not-string", "\n";
echo (string)true, ":", (string)false, ":done\n";

echo (bool)"" ? "t" : "f";
echo (bool)"0" ? "t" : "f";
echo (bool)"false" ? "t" : "f";
echo (bool)0 ? "t" : "f";
echo (bool)1 ? "t" : "f";
echo (bool)array() ? "t" : "f";
echo (bool)array(0) ? "t" : "f";
echo "\n";
echo is_bool((bool)1) ? "bool" : "not-bool", "\n";

$wrapped = (array)"single";
echo json_encode($wrapped), ":", count($wrapped), "\n";
$kept = (array)array("a" => 1);
echo json_encode($kept), ":", is_array($kept) ? "array" : "not-array", "\n";
---
4
12
0
3
-3
1:0
int
2.5
7
42
3.5
string
1::done
fftftft
bool
["single"]:1
{"a":1}:array
