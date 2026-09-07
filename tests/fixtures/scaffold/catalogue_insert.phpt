name: writing to the seeded table
serial: true
runner:
  php: false
description: >
  A fixture that writes to the seeded table and removes what it wrote, so the
  peers reading the seed still read three rows. It is serial because it changes
  the state they share; php cannot run it because Database is a binding.
---
<?php

$db = new Database("scaffold");

$db->insert("catalogue", array("name" => "Hopper", "category" => "People"));
echo $db->get("select count(id) as total from catalogue")["total"] . "\n";

$row = $db->get("select name, category from catalogue where name = ?", "Hopper");
echo $row["name"] . " (" . $row["category"] . ")\n";

$db->query("delete from catalogue where name = 'Hopper'");
echo $db->get("select count(id) as total from catalogue")["total"] . "\n";
?>
---
4
Hopper (People)
3
