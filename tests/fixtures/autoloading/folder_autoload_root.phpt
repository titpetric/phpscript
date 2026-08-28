name: autoload folder root holds unnamespaced classes
description: >
  The namespace in the autoload rule is optional: a class declared in no
  namespace resolves at the root of the folder, so Greeter is
  autoload/Greeter.php. php has no equivalent convention.
runner:
  php: false
---
<?php

$greeter = new Greeter();
echo $greeter->greet("world") . "\n";
?>
---
hello, world
