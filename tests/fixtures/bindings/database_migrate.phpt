name: database migrations
runner:
  php: false
description: Database\Migrate loads and runs SQL migrations against a named sqlite database.
---
<?php

$migrate = new Database\Migrate("sqlite_test");
$migrate->load("./schema/*.sql");
$migrate->run();

$db = new Database("sqlite_test");
$row = $db->get("select name from migration_users where id = ?", 1);
echo $row["name"] . "\n";
---
Ada
