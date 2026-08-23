name: array_search and in_array strictness
description: >
  array_search returns the key rather than the offset, so it is correct for a
  string-keyed array, and false when there is no match. Key 0 and false are only
  told apart with ===. Under PHP 8 a non-numeric string does not equal 0, and
  the $strict parameter compares types as well as values.
---
<?php
var_dump(array_search("y", array("x", "y")));
var_dump(array_search("q", array("x", "y")));
var_dump(array_search("b", array("x" => "a", "y" => "b")));
var_dump(array_search(0, array("a", "b")));
var_dump(array_search("1", array(1, 2)));
var_dump(array_search("1", array(1, 2), true));
var_dump(in_array("1", array(1, 2)));
var_dump(in_array("1", array(1, 2), true));
var_dump(in_array(0, array("a", "b")));
---
int(1)
bool(false)
string(1) "y"
bool(false)
int(0)
bool(false)
bool(true)
bool(false)
bool(false)
