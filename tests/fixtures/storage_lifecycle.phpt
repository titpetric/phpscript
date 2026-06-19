name: storage lifecycle
description: >
  `new Storage` constructs a Go-backed value via the registered constructor
  (with the context auto-injected). get() returns a rich Record struct; its
  fields are read with `->` (case-insensitive) before printing.
---
<?php
$storage = new Storage;
$storage->set("greeting", "hello");
$storage->set("name", "world");
$greeting = $storage->get("greeting");
$name = $storage->get("name");
$count = $storage->len();
echo $greeting->key . "=" . $greeting->value . ", " . $name->value . "! count=" . $count;
---
greeting=hello, world! count=2
