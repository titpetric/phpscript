name: get_included_files
description: get_included_files returns included dirFS filenames.
---
<?php
include("code/Compiler.php");
include("./code/Template.php");
$files = get_included_files();
echo $files[0] . "\n" . $files[1];
---
code/Compiler.php
code/Template.php
