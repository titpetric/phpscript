name: storage method error (caught)
runner:
  php: false
description: >
  A method's error can be caught with try/catch. The caught exception is bound
  to $e, and `echo $e` prints its message — so execution continues normally and
  the host produces output rather than an Internal Server Error.
---
<?php
$storage = new Storage;
try {
    $value = $storage->get("missing");
    echo $value;
} catch (Exception $e) {
    echo "caught: " . $e;
}
---
caught: storage: missing key missing
