name: the go runtime refuses array()
description: >
  The go runtime spells its composite literals with brackets and braces, and
  array() is not one of them: `[]` builds a slice and `{key: value}` a map. A
  file that writes array() fails the parse, before the body runs.
runtime: go
error: the go runtime has no array()
---
<?php

$a = array(1, 2);
echo len($a), "\n";
---
Internal Server Error
