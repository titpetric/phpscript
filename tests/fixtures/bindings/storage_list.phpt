name: storage list of rich types
runner:
  php: false
description: >
  all() returns a Go slice of rich Record structs. PHP foreach iterates the Go
  slice directly, and each element's struct fields are read with `->`. This
  exercises foreach over a native Go slice (not a PHP array) and per-element
  struct field access.
---
<?php
$storage = new Storage;
$storage->set("a", "1");
$storage->set("b", "2");
$storage->set("c", "3");
$records = $storage->all();
foreach ($records as $r) {
    echo $r->key . "=" . $r->value . ";";
}
---
a=1;b=2;c=3;
