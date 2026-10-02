name: preg offset capture composes with substr
description: PREG_OFFSET_CAPTURE reports byte offsets, and php's substr and
  strlen count bytes too, so an offset feeds straight back into a slice. The
  subject carries a two-byte character before the match to pin the unit.
---
<?php
$s = "a·b{x}c";

preg_match("/\{[^{]+\}/", $s, $m, PREG_OFFSET_CAPTURE);
list($tag, $offset) = $m[0];

echo strlen($s), " ", $offset, "\n";
echo substr($s, 0, $offset), "\n";
echo substr($s, $offset + strlen($tag)), "\n";
---
8 4
a·b
c
