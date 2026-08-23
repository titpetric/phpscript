name: a fixture can root itself at a real directory
description: >
  A fixture names an include root with `root:`, resolved against its own
  directory, and the runtime reads that tree from disk instead of the embedded
  one. That is what lets a fixture load a tree phpscript does not embed, a
  composer vendor directory being the case it exists for.
root: tree
---
<?php
require 'lib/greeting.php';
echo tree_greeting(), "\n";
---
loaded from the real filesystem
