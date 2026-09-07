name: seeded row lookup
runner:
  php: false
description: >
  A parameterised get() against a row the setup hook seeded, and the empty
  result for one it did not. php cannot run it because Database is a binding.
---
<?php

$db = new Database("scaffold");

$row = $db->get("select id, name from catalogue where name = ?", "Grace");
echo $row["name"] . "#" . $row["id"] . "\n";

$missing = $db->get("select id from catalogue where name = ?", "Hopper");
echo ($missing ? "found" : "none") . "\n";
?>
---
Grace#2
none
