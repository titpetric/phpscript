name: http_build_query
description: >
  http_build_query urlencodes both halves of every pair, spells a nested array
  as key[sub]=value, writes booleans as 1 and 0 and drops a null entry.
---
<?php
echo http_build_query(["name" => "Tit Petric", "lang" => "en~us", "n" => 5]), "\n";
echo http_build_query(["filter" => ["field" => "name", "op" => "="], "page" => 2]), "\n";
echo http_build_query(["tags" => ["a", "b"], "deep" => ["x" => ["y" => "z"]]]), "\n";
echo http_build_query([1, 2, 3]), "\n";
echo http_build_query(["on" => true, "off" => false, "skip" => null, "f" => 1.5]), "\n";
var_dump(http_build_query([]));
---
name=Tit+Petric&lang=en%7Eus&n=5
filter%5Bfield%5D=name&filter%5Bop%5D=%3D&page=2
tags%5B0%5D=a&tags%5B1%5D=b&deep%5Bx%5D%5By%5D=z
0=1&1=2&2=3
on=1&off=0&f=1.5
string(0) ""
