name: get_included_files
description: get_included_files returns included dirFS filenames.
---
<?php
include("minitpl/Compiler.php");
include("./minitpl/Template.php");
$files = get_included_files();
echo $files[0] . "\n" . $files[1];
---
minitpl/Compiler.php
minitpl/Template.php
