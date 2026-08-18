name: storage method error (uncaught)
runner:
  php: false
description: >
  A method's trailing Go error (omitted from the PHP assignment) is surfaced as
  a thrown error. Uncaught, it aborts execution and the host renders an
  "Internal Server Error" instead of the partial output.
error: missing key
---
<?php
$storage = new Storage;
$value = $storage->get("missing");
echo $value;
---
Internal Server Error
