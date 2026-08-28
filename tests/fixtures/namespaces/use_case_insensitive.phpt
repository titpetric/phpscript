name: use alias is case-insensitive
description: >
  PHP reads a class name in any case, and a `use` import does not change
  that: `use App\Support\SomeClass;` answers someclass::get(), SOMECLASS
  and new someclass() alike. get_class() still reports the declared
  spelling, not the one the call site typed.
---
<?php

require "usecase/someclass.php";

use App\Support\SomeClass;

echo SomeClass::get(), "\n";
echo someclass::get(), "\n";
echo SOMECLASS::get(), "\n";
echo SoMeClAsS::KIND, "\n";

$a = new someclass("lower");
echo $a->label(), "\n";
echo get_class($a), "\n";
---
SomeClass::get
SomeClass::get
SomeClass::get
support
lower (support)
App\Support\SomeClass
