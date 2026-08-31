name: redeclaring a function
runner:
  php: false
description: >
  A function declared twice, and a function declared over a name the runtime
  already registers, are both refused where the declarations are hoisted. Each
  one reaches the script as a catchable Exception carrying php's own message, so
  an include that redeclares is answerable rather than fatal, and the program
  keeps running: the first declaration of widget() stands, and strlen() is still
  the binding. PHP reports the same condition as a compile-time fatal error that
  no catch can reach, which is why only phpscript defines the expected output.
  The support files are included rather than written inline because a
  declaration in this file would be hoisted before the try around it runs.
---
<?php

try {
    include "redeclared_function.php";
    echo "no error;";
} catch (Exception $e) {
    echo get_class($e), ": ", $e->getMessage(), ";";
}

try {
    include "redeclared_binding.php";
    echo "no error;";
} catch (Exception $e) {
    echo get_class($e), ": ", $e->getMessage(), ";";
}

echo function_exists("widget") ? "widget declared;" : "widget missing;";
echo widget(), ";";
echo strlen("abc"), ";";
echo "still running\n";
?>
---
Exception: Cannot redeclare function widget() (previously declared in redeclared_function.php:3);Exception: Cannot redeclare function strlen();widget declared;first;3;still running
