name: magic path constants
description: >
  __FILE__ and __DIR__ resolve at the top level of a script, not only inside an
  included file. The fixture asserts the directory name rather than the file
  name, because the php runner executes a temporary copy of the source.
---
<?php
echo basename(__DIR__), "\n";
echo __FILE__ !== "" ? "FILE set\n" : "FILE empty\n";
echo __DIR__ !== "" ? "DIR set\n" : "DIR empty\n";
---
paths
FILE set
DIR set
