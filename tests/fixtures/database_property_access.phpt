name: database binding property access
flatstack: true
description: >
  A property of a Go binding resolves the way its methods do: $db->is_readonly
  reads and writes the field IsReadonly, folding case and underscores the same
  way get_all() resolves GetAll. An unexported field is not a property — it
  reads as nothing rather than through reflection.
---
<?php

$db = new Database("sqlite_test");

echo "readonly: " . ($db->is_readonly ? "yes" : "no") . "\n";

$db->is_readonly = true;
echo "readonly: " . ($db->is_readonly ? "yes" : "no") . "\n";

$db->is_readonly = false;
echo "readonly: " . ($db->is_readonly ? "yes" : "no") . "\n";

echo "unexported: [" . $db->transaction . "]\n";

$db->close();
---
readonly: no
readonly: yes
readonly: no
unexported: []
