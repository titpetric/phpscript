name: flatstack runner-compatible fast path
runner:
  php: false
description: >
  A Go-backed constructor, context injection, method calls, and returned struct
  property access all execute through native flat bytecode.
---
<?php
$storage = new Storage;
$storage->set("color", "blue");
$record = $storage->get("color");
echo $storage->tenant() . ":" . $record->value;
---
acme:blue
