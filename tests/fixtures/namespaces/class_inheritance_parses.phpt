name: extends and implements parse
description: >
  A namespaced file whose class carries both `extends` and `implements`. Both
  clauses are parsed and recorded on the AST so the file lints and reformats,
  and neither confers anything: the class is exercised only through the
  constant and the methods it declares itself, because phpscript inherits no
  members from a parent and checks no interface.
---
<?php

require "inherit/registry.php";

use App\Registry\Counter;

$counter = new Counter("hits");

echo $counter->name(), "\n";
echo $counter->count(), "\n";
$counter->add("a");
$counter->add("b");
echo $counter->count(), "\n";
echo Counter::KIND, "\n";
echo $counter->describe(), "\n";
---
hits
0
2
counter
counter hits
