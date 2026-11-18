name: an import binds the file that writes it and reaches no other file
description: >
  `import "strings"` names a package in the file it sits in. A file the entry
  includes parses on its own, so it writes its own import line; the entry's
  import leaves the name unbound there, and the same call reads as php's
  concatenation. Reaching the area by its full name needs no import in either
  file. php has no import statement and no Runtime namespace, so only
  phpscript defines this output.
runner:
  php: false
---
<?php

use Runtime\Go;

import "strings";

echo strings.ToUpper("entry"), "\n";

include "import_per_file_helper.php";

// The area answers under its own name in every file.
echo Runtime\Go\Strings\ToLower("AREA"), "\n";
---
ENTRY
helper: no import
helper: S
area
