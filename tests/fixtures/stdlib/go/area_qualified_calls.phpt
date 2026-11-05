name: a function is callable through the binding area that defines it
description: >
  phpscript groups its bindings into the Runtime\PS namespace, one area per
  stdlib folder, and phpx overlays them as Runtime\PX. A call written through
  the area reaches the global binding, in any case, and function_exists agrees.
  php has no such namespace, so only phpscript defines the expected output.
runner:
  php: false
---
<?php

echo \Runtime\PS\Core\Strings\strlen("four"), "\n";
echo Runtime\PS\Core\Strings\STRTOUPPER("a"), "\n";
var_dump(function_exists('Runtime\PS\Core\Strings\strlen'));
var_dump(function_exists('Runtime\PS\Core\Arrays\strlen'));
var_dump(function_exists('Runtime\PS\Nope\strlen'));
---
4
A
bool(true)
bool(false)
bool(false)
