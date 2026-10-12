name: include falls back to the calling script's directory
description: >
  A relative include that does not start with ./ or ../ is looked for from the
  working directory first and in the directory of the calling script after,
  so a script can include its own neighbours from wherever it is run. A file
  in the working directory still wins over one beside the calling script.
---
<?php
include "fallback/entry.php";
---
helper beside the calling script
counter ran
