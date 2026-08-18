name: get_included_files
runner:
  php: false
description: >
  get_included_files returns included dirFS filenames. The list names the
  runtime root filesystem, so only phpscript defines the expected output.
---
<?php
include("modules/menu.php");
include("./modules/functions.php");
$files = get_included_files();
echo $files[0] . "\n" . $files[1];
---
modules/menu.php
modules/functions.php
