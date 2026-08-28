name: autoload folder resolves a namespaced class
description: >
  An autoload/ directory at the application root loads a class on first
  reference, with no include and no spl_autoload_register. The namespace is the
  directory path below it, case for case, so Fixture\Loaded is
  autoload/Fixture/Loaded.php. php has no equivalent convention.
runner:
  php: false
---
<?php

echo class_exists("Fixture\\Loaded", false) ? "loaded\n" : "not loaded\n";

$loaded = new Fixture\Loaded("the autoload folder");
echo $loaded->message() . "\n";
echo class_exists("Fixture\\Loaded", false) ? "loaded\n" : "not loaded\n";
?>
---
not loaded
loaded by the autoload folder
loaded
