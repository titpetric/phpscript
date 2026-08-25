name: include_once and require_once
description: >
  The *_once keywords execute a file only if it has not run yet, and the record
  they consult covers every include: a plain include earlier in the file
  already satisfies a later include_once, and include_once and require_once
  share the record with each other. Plain include runs the file every time it
  is evaluated.
---
<?php

include "counter.php";
include "counter.php";
echo "after include: ", $counter_runs, "\n";

include_once "counter.php";
include_once "counter.php";
require_once "counter.php";
echo "after once: ", $counter_runs, "\n";

include "counter.php";
echo "after include again: ", $counter_runs, "\n";
---
counter ran
counter ran
after include: 2
after once: 2
counter ran
after include again: 3
