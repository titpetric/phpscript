name: Using include statements to get data returned from the php file.
description: >
  The include function can be used to include a file that returns a value.
  The returned value can be assigned to a variable. It is an essential
  hot loading property of include that enables on the fly logic changes,
  effectively building a hot-loading plugin system.
---
<?php

$message = include("plugin.php");

echo $message . "\n";

?>
---
Hello world!
