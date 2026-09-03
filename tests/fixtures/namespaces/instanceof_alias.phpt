name: instanceof resolves its right operand through a use alias
description: >
  A bare class name on the right of instanceof goes through the same name
  resolution as new, a static call, an implements list and ::class, so a
  `use` alias answers the check the way the declared name does. get_class
  and Baz::class report the declared name; both spellings of the check are
  true. Guards the fix for
  https://github.com/titpetric/phpscript/issues/84.
---
<?php

class Bar
{
    public $x = 1;
}

use Bar as Baz;

$b = new Baz();

// The alias resolves everywhere a class name is qualified at parse time.
// `new` reaches the class, and `::class` reports its real name.
var_dump(get_class($b));
var_dump(Baz::class);
var_dump(get_class($b) === Baz::class);

// instanceof against the name the class declared.
var_dump($b instanceof Bar);

// The same name resolution on the right operand of instanceof.
var_dump($b instanceof Baz);
?>
---
string(3) "Bar"
string(3) "Bar"
bool(true)
bool(true)
bool(true)
