name: database binding property access
serial: true
runner:
  php: false
description: >
  A property of a Go binding resolves the way its methods do: $db->is_readonly
  reads and writes the field IsReadonly, folding case and underscores the same
  way get_all() resolves GetAll. An unexported field is not a property: it
  reads as nothing rather than through reflection.

  get_object_vars and property_exists answer off the same field set, which is
  what a script can read off a client. The listing is pinned because it is a
  disclosure surface: an exported field added to Database is a decision, and
  docs/security.md records that neither of these two carries a credential or
  reaches past the read-only flag.
---
<?php

$db = new Database("sqlite_test");

echo "readonly: " . ($db->is_readonly ? "yes" : "no") . "\n";

$db->is_readonly = true;
echo "readonly: " . ($db->is_readonly ? "yes" : "no") . "\n";

$db->is_readonly = false;
echo "readonly: " . ($db->is_readonly ? "yes" : "no") . "\n";

echo "unexported: [" . $db->transaction . "]\n";

echo "properties: " . implode(",", array_keys(get_object_vars($db))) . "\n";
echo "declared: " . (property_exists($db, "is_readonly") ? "yes" : "no") . "\n";
echo "unexported: " . (property_exists($db, "transaction") ? "yes" : "no") . "\n";
echo "bridge: " . (property_exists($db, "bridge") ? "yes" : "no") . "\n";

$db->close();
---
readonly: no
readonly: yes
readonly: no
unexported: []
properties: ID,IsReadonly
declared: yes
unexported: no
bridge: no
