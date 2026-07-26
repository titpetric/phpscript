name: default spl autoloading
description: >
  Registering spl_autoload without a callback searches the configured include
  path for the lowercased namespaced class name with PHP file extensions.
---
<?php

set_include_path("autoload-default");
spl_autoload_register();

$loaded = new DefaultFixture\Loaded();
echo $loaded->message() . "\n";
?>
---
loaded by default spl_autoload
