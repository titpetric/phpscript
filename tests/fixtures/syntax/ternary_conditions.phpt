name: ternary conditions
description: >
  Covers full and shorthand ternary expressions, including functions loaded from
  code/functions.php and direct invocations of compatible functions.
---
<?php
include "code/functions.php";

$a = "left";
$b = "right";
echo ($a ?: $b) . ";";
echo ($a ? $a : $b) . ";";

$a = "";
echo ($a ?: $b) . ";";
echo ($a ? $a : $b) . ";";

echo issetor("", "fallback") . ";";
echo issetor(false, array("x")) ? "array;" : "missing;";

echo is_email("test@example.com") ? "email;" : "bad;";
echo is_email("bad address@example.com") ? "bad;" : "invalid;";

$kept = array_keep(array("a" => "A", "b" => "B"), "b");
echo $kept["b"];
---
left;left;right;right;fallback;array;email;invalid;B
