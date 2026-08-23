name: output buffering
description: >
  ob_start captures everything echo would emit, nests, and hands the text back
  through ob_get_contents/ob_get_clean. This is how a template engine renders to
  a string rather than to the response.
---
<?php

echo "before;";

ob_start();
echo "captured;";
$level = ob_get_level();
$seen = ob_get_contents();
ob_end_clean();

echo "level=" . $level . ";";
echo "seen=" . $seen;

ob_start();
echo "outer(";
ob_start();
echo "inner";
$inner = ob_get_clean();
echo $inner . ");";
echo ob_get_clean();

echo "after=" . ob_get_level() . ";";
// With no buffer active ob_get_contents reports false, as PHP does.
echo ob_get_contents() === false ? "no-buffer" : "buffer";
?>
---
before;level=1;seen=captured;outer(inner);after=0;no-buffer
