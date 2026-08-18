name: string arguments coerce the way PHP renders values
description: >
  A value passed where a binding declares a string parameter is rendered the
  way PHP renders it in a string context, so strlen(65) measures "65" and not
  Go's conversion of the code point 65 to "A". The expected output is what
  php 8.4 prints for this source.
---
<?php

echo strlen(65), "\n";
echo strtoupper(65), "\n";
echo str_repeat(5, 3), "\n";
echo trim(65), "\n";
echo substr(12345, 1, 2), "\n";
echo strlen(1.5), "\n";
echo strtoupper(true), "\n";
echo strlen(false), "\n";
echo str_replace(1, 2, 121), "\n";
?>
---
2
65
555
65
23
3
1
0
222
