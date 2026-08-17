name: get_included_files
description: get_included_files returns included dirFS filenames.
---
<?php
include("modules/menu.php");
include("./code/functions.php");
$files = get_included_files();
echo $files[0] . "\n" . $files[1];
---
modules/menu.php
code/functions.php
