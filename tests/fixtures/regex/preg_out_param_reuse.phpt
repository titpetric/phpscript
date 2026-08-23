name: preg out parameter reuse
description: >
  A by-reference out parameter overwrites a variable that already holds a
  value. Reusing $matches across calls, the way a loop does, has to see the
  current call's matches rather than the first call's; a fresh variable is the
  case that works either way.
---
<?php
preg_match_all("/a/", "aa", $m);
echo count($m[0]), "\n";
preg_match_all("/b/", "bbb", $m);
echo count($m[0]), "\n";
preg_match_all("/b/", "bbb", $other);
echo count($other[0]), "\n";
---
2
3
3
