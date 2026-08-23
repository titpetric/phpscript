name: var_export
description: >
  var_export emits valid PHP source: two-space indent, a space between array and
  its paren, a trailing comma on every element, single-quoted keys, and NULL in
  capitals. A nested array follows a trailing space after its arrow. A float
  always carries a fraction, so 1.0 does not read back as an integer.
---
<?php
echo var_export(array(1, "a" => true), true), "\n";
echo var_export(array("a" => array("b" => 1), "c" => 1.0, "d" => null), true), "\n";
echo var_export("it's a \\ backslash", true), "\n";
echo var_export(false, true), "\n";
var_export(array(2));
echo "\n";
---
array (
  0 => 1,
  'a' => true,
)
array (
  'a' => 
  array (
    'b' => 1,
  ),
  'c' => 1.0,
  'd' => NULL,
)
'it\'s a \\ backslash'
false
array (
  0 => 2,
)
