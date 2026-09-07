name: seeded row counts
runner:
  php: false
description: >
  Aggregates over the seeded table, which is the state the setup hook left and
  not something this fixture built. php cannot run it because Database is a
  binding.
---
<?php

$db = new Database("scaffold");

echo $db->get("select count(id) as total from catalogue")["total"] . "\n";

foreach ($db->get_all("select category, count(id) as total from catalogue group by category order by category") as $row) {
	echo $row["category"] . "=" . $row["total"] . "\n";
}
?>
---
3
Equipment=1
People=2
