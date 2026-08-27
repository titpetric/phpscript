name: parse_str decodes bracket syntax into nested arrays
description: >
  The expected section is php's own output, so the matrix run compares the
  decoder against the reference implementation.
---
<?php

parse_str("a[b]=1&a[c][]=2&a[c][]=3&plain=x&last=1&last=2", $out);
var_dump($out["a"]["b"]);
var_dump($out["a"]["c"][0]);
var_dump($out["a"]["c"][1]);
var_dump($out["plain"]);
var_dump($out["last"]);
---
string(1) "1"
string(1) "2"
string(1) "3"
string(1) "x"
string(1) "2"
