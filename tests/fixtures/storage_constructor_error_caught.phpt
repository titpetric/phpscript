name: storage constructor error (caught)
runner:
  php: false
description: >
  A failing constructor can be caught with try/catch. `new FailStorage` raises,
  the error is bound to $e, and `echo $e` prints its message; execution then
  continues past the try block.
---
<?php
try {
    $storage = new FailStorage;
    echo "unreachable";
} catch (Exception $e) {
    echo "caught: " . $e;
}
---
caught: boom
