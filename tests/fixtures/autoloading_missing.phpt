name: undefined class without autoloader
flatstack: true
description: >
  No class autoloader exists unless spl_autoload_register is invoked, so an
  undefined class construction throws an error that PHP code can catch.
---
<?php

try {
    new NonExistent;
} catch (Error $error) {
    echo "undefined class\n";
}
?>
---
undefined class
