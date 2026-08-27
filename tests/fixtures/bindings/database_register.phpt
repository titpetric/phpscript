name: database register
description: Database::register adds a connection new Database() can then open.
serial: true
runner:
  php: false
---
<?php
try {
	$db = new Database("never-registered");
	echo "opened an unregistered name\n";
} catch (Exception $e) {
	echo $e->getMessage(), "\n";
}

echo Database::register("scratch", "sqlite://:memory:") ? "registered" : "refused", "\n";
echo in_array("scratch", Database::connections()) ? "listed" : "missing", "\n";

// The provider caches one pool per name for the life of the process, so the
// fixture starts from a known table rather than from an empty database.
$db = new Database("scratch");
$db->query("CREATE TABLE IF NOT EXISTS note (id INTEGER PRIMARY KEY, body TEXT)");
$db->query("DELETE FROM note");
$db->insert("note", array("body" => "written through a registered connection"));

$row = $db->get("SELECT body FROM note WHERE id = ?", $db->insert_id());
echo $row["body"], "\n";

// Registering the same name with the same DSN is a no-op, so the row the
// previous handle wrote is still there.
Database::register("scratch", "sqlite://:memory:");
$same = new Database("scratch");
echo count($same->get_all("SELECT id FROM note")), "\n";
---
no configuration found for database: [never-registered]
registered
listed
written through a registered connection
1
