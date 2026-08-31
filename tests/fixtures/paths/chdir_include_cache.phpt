name: an include is cached under the path it resolved to, not the path it was written as
description: >
  "samename.php" names two different files depending on where the working
  directory is, so including it from both places has to produce both. The parsed
  include is cached, and a cache keyed by the path as the script wrote it would
  answer the second include with the first file - the same spelling, the wrong
  contents. This fixture is that case: it fails if the key ever stops carrying
  the working directory.
---
<?php

$root = include "samename.php";
var_dump(chdir("workdir"));
$moved = include "samename.php";
var_dump(chdir(".."));
$back = include "samename.php";

echo $root, "|", $moved, "|", $back, "\n";

// The second include of a path already resolved is the cached one, and it is
// still the right file: caching is what this asserts, not that it was skipped.
$again = include "workdir/samename.php";
echo $again, "\n";
?>
---
bool(true)
bool(true)
root|workdir|root
workdir
