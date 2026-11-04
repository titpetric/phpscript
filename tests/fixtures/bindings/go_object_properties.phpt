name: properties of a value a binding returned
runner:
  php: false
description: >
  get_object_vars and property_exists answer for a value a binding returned the
  way property access does. `Storage::get` hands over a Record struct, its
  exported fields are the properties a script reads, and property_exists
  resolves a name the way `->` resolves it: case-insensitively.
---
<?php
$storage = new Storage;
$storage->set("greeting", "hello");
$record = $storage->get("greeting");

var_export(property_exists($record, "Key"));
echo "\n";
var_export(property_exists($record, "value"));
echo "\n";
var_export(property_exists($record, "absent"));
echo "\n";
foreach (get_object_vars($record) as $name => $value) {
    echo $name . "=" . $value . "\n";
}
---
true
true
false
Key=greeting
Value=hello
