name: a declared DATETIME column survives the round trip to echo
description: >
  The driver scans a DATETIME column into a Go time.Time. Echoing it must
  produce the stored text, the same thing php prints when PDO returns the
  column as a string: php refuses to convert a DateTime object to a string at
  all, and every datetime php writes itself is Y-m-d H:i:s. Needs the
  sqlite_test credential from .env.testing.
runner:
  php: false
---
<?php

// The sqlite_test connection is shared in-memory, and the matrix runs this
// fixture once per runtime, so the table is rebuilt rather than added to.
$db = new Database("sqlite_test");
$db->query("DROP TABLE IF EXISTS probe");
$db->query("CREATE TABLE probe (id INTEGER PRIMARY KEY, created_at DATETIME NOT NULL DEFAULT '2026-08-26 14:48:00')");
$db->query("INSERT INTO probe (id) VALUES (1)");

$row = $db->get("SELECT created_at FROM probe");
echo $row["created_at"], "\n";
---
2026-08-26 14:48:00
