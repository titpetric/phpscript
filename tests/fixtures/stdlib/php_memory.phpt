name: php://memory and php://temp hold a buffer a script owns
description: >
  The handle API over a buffer with no file behind it: writes extend it, seeks
  move it, feof turns true on the read that comes back empty, and
  stream_get_contents honours its length and offset. An append mode writes at
  the end wherever the handle sits, where "r" and "c" refuse the write.
  php://temp takes the maxmemory suffix and opens the same buffer. The
  whole-file helpers open a fresh buffer each call: file_get_contents reads the
  empty string and file_put_contents counts the bytes it then drops.
---
<?php

$m = fopen("php://memory", "w+");
echo fwrite($m, "abc"), fwrite($m, "def"), "\n";
echo ftell($m), "\n";
rewind($m);
echo fread($m, 3), "\n";
var_dump(feof($m));
echo fgets($m), "\n";
var_dump(feof($m));
var_dump(fread($m, 3));
fseek($m, 1, SEEK_SET);
var_dump(feof($m));
echo stream_get_contents($m), "\n";
echo stream_get_contents($m, 3, 0), "\n";
echo stream_get_contents($m, -1, 2), "\n";
echo ftell($m), "\n";
var_dump(fclose($m));

$gap = fopen("php://memory", "w+");
fwrite($gap, "ab");
fseek($gap, 5);
fwrite($gap, "z");
echo bin2hex(stream_get_contents($gap, -1, 0)), "\n";

$ap = fopen("php://memory", "a");
fwrite($ap, "abc");
rewind($ap);
fwrite($ap, "X");
echo stream_get_contents($ap, -1, 0), "\n";

$ro = fopen("php://memory", "r");
var_dump(fwrite($ro, "x"));
var_dump(stream_get_contents($ro));
var_dump(fwrite(fopen("php://memory", "c"), "x"));

$t = fopen("php://temp/maxmemory:16", "w+");
fwrite($t, "spilled");
rewind($t);
echo stream_get_contents($t), "\n";
fclose($t);

var_dump(file_get_contents("php://memory"));
var_dump(file_put_contents("php://temp", "dropped"));
echo file_put_contents("php://output", "out\n"), "\n";
---
33
6
abc
bool(false)
def
bool(true)
string(0) ""
bool(false)
bcdef
abc
cdef
6
bool(true)
61620000007a
abcX
bool(false)
string(0) ""
bool(false)
spilled
string(0) ""
int(7)
out
4
