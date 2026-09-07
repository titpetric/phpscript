name: seeded rows
runner:
  php: false
description: >
  The rows the suite's setup hook inserted are there before the fixture runs.
  The connection is the one this folder's phpscript.yml named, so nothing about
  it is on the command line; php cannot run it because Database is a binding.
---
<?php

$db = new Database("scaffold");

foreach ($db->get_all("select name, category from catalogue order by id") as $row) {
	echo $row["name"] . " (" . $row["category"] . ")\n";
}
?>
---
Ada (People)
Grace (People)
Desk Lamp (Equipment)
