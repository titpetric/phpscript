name: substr_count and substr_replace
description: >
  substr_count counts non-overlapping occurrences and honours an $offset.
  substr_replace takes a negative $offset from the end, and a negative $length
  as a distance from the end rather than a count.
---
<?php
var_dump(substr_count("hello world", "o"));
var_dump(substr_count("hello world", "o", 5));
var_dump(substr_count("aaa", "aa"));
var_dump(substr_replace("Hello world", "PHP", 6, 5));
var_dump(substr_replace("Hello", "X", -3, -1));
var_dump(substr_replace("Hello", "X", 1));
---
int(2)
int(1)
int(1)
string(9) "Hello PHP"
string(4) "HeXo"
string(2) "HX"
