name: join aliases implode
description: >
  join is PHP's alias of implode: values joined with the separator, the
  single-array form joining with an empty string.
---
<?php

echo join(",", array("a", "b", "c")), "\n";
echo join(array("x", "y")), "\n";
var_dump(join("-", array()));
var_dump(join("|", array(1, 2, 3)));
---
a,b,c
xy
string(0) ""
string(5) "1|2|3"
