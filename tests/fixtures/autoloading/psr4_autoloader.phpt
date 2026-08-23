name: psr-4 class loader
description: >
  A PSR-4 class loader built from the parts a generated autoloader is made of:
  a static bootstrap method, a static property holding the include closure, an
  autoload callback registered as array($this, "loadClass"), and a findFile that
  walks the namespace with `while (false !== $pos = strrpos(...))`. Registering
  it makes a namespaced class resolvable; unregistering takes that away again.
---
<?php

require "psr4/loader.php";

use Acme\Loader\ClassLoader;

$loader = ClassLoader::bootstrap("Acme\\Greeting\\", "psr4/Acme/Greeting");

echo $loader->findFile("Acme\\Greeting\\Formal") . "\n";
echo $loader->findFile("Acme\\Missing\\Thing") === false ? "not found\n" : "found\n";

$formal = new Acme\Greeting\Formal();
echo $formal->greet("world") . "\n";
echo Acme\Greeting\Formal::SALUTATION . "\n";

echo ClassLoader::bootstrap("Acme\\Other\\", "nowhere") === $loader ? "same\n" : "new\n";

$loader->unregister();
echo class_exists("Acme\\Greeting\\Absent") ? "loaded\n" : "not loaded\n";
---
psr4/Acme/Greeting/Formal.php
not found
Good day, world.
Good day
same
not loaded
