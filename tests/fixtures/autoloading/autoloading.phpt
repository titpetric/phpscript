name: namespaces and callback autoloading
description: >
  A registered callback receives the fully-qualified class name, includes its
  namespaced declaration, and class_exists can inspect without autoloading.
---
<?php

spl_autoload_register(function ($class) {
    echo "autoload:" . $class . "\n";
    include "autoload/" . str_replace("\\", "/", $class) . ".php";
});

echo class_exists("Fixture\\Loaded", false) ? "loaded\n" : "not loaded\n";

$loaded = new Fixture\Loaded("callback");
echo $loaded->message() . "\n";
echo class_exists("Fixture\\Loaded", false) ? "loaded\n" : "not loaded\n";
?>
---
not loaded
autoload:Fixture\Loaded
loaded by callback
loaded
