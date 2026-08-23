name: preg_replace_callback
description: >
  preg_replace_callback invokes the callable once per match in document order
  with the match array as its argument, honours $limit, and reports how many
  replacements it made through the by-reference $count.
---
<?php
echo preg_replace_callback("/\d+/", function ($m) { return $m[0] * 2; }, "a1b20"), "\n";
$count = 0;
echo preg_replace_callback("/\d/", function ($m) { return "#"; }, "a1b2", 1, $count), "\n";
echo $count, "\n";
echo preg_replace_callback("/(\w)(\d)/", function ($m) { return $m[2] . $m[1]; }, "a1b2"), "\n";
---
a2b40
a#b2
1
1a2b
