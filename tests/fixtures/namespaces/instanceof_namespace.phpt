name: instanceof resolves its right operand through the current namespace
description: >
  Inside `namespace Foo`, a bare `Bar` on the right of instanceof means
  `Foo\Bar`, the same relative resolution every other class-name site gets,
  and the fully qualified spelling agrees with it. From the global
  namespace, `use Foo\Bar` resolves the bare name the same way. Guards the
  fix for https://github.com/titpetric/phpscript/issues/84.
---
<?php

require "instanceofns/lib.php";

use Foo\Bar;

Foo\check();

// An import from the global namespace resolves the bare name too.
$b = new Bar();
var_dump($b instanceof Bar);
var_dump($b instanceof \Foo\Bar);
?>
---
bool(true)
bool(true)
bool(true)
bool(true)
