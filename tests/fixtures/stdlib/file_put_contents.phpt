name: file_put_contents writes, appends and reports length
description: >
  Round trip through file_get_contents, then FILE_APPEND, then a flag pair
  with LOCK_EX. Runs under php unchanged; assumes a writable working
  directory.
---
<?php

$f = "fixture-write.txt";
var_dump(file_put_contents($f, "one\n"));
var_dump(file_put_contents($f, "two\n", FILE_APPEND));
var_dump(file_put_contents($f, "three\n", FILE_APPEND | LOCK_EX));
echo file_get_contents($f);
var_dump(file_put_contents($f, "over\n"));
echo file_get_contents($f);
unlink($f);
---
int(4)
int(4)
int(6)
one
two
three
int(5)
over
