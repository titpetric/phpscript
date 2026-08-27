name: database read-only client
serial: true
runner:
  php: false
description: >
  $db->is_readonly restricts a client to statements that only read, and refuses
  everything else. The CRUD helpers are refused before a statement is built;
  query(), get() and get_all() are refused on the keyword the statement starts
  with, past any comment tagging it. The property is request scope: the script
  that set it can clear it, and the client writes again.
---
<?php

$db = new Database("sqlite_test");
$db->query("drop table if exists readonly_users");
$db->query("create table readonly_users (id integer primary key autoincrement, name text)");
$db->insert("readonly_users", array("name" => "Ada"));

echo "default: " . ($db->is_readonly ? "yes" : "no") . "\n";

$db->is_readonly = true;
echo "readonly: " . ($db->is_readonly ? "yes" : "no") . "\n";

$row = $db->get("/* userGet */ select id, name from readonly_users where name = ?", "Ada");
echo $row["name"] . "#" . $row["id"] . "\n";

foreach ($db->get_all("select name from readonly_users order by id") as $user) {
	echo $user["name"] . "\n";
}

try {
	$db->insert("readonly_users", array("name" => "Grace"));
} catch (Exception $e) {
	echo "insert: " . $e . "\n";
}

try {
	$db->update("readonly_users", array("id" => 1, "name" => "Grace"), "id");
} catch (Exception $e) {
	echo "update: " . $e . "\n";
}

try {
	$db->query("/* purge */ delete from readonly_users");
} catch (Exception $e) {
	echo "query: " . $e . "\n";
}

try {
	$db->get_all("drop table readonly_users");
} catch (Exception $e) {
	echo "get_all: " . $e . "\n";
}

// The refusals left the table as it was built.
$users = $db->get_all("select name from readonly_users");
echo "rows: " . count($users) . "\n";

// The property is the whole boundary, and it is request scope.
$db->is_readonly = false;
$db->insert("readonly_users", array("name" => "Grace"));
echo "rows: " . count($db->get_all("select name from readonly_users")) . "\n";

$db->close();
---
default: no
readonly: yes
Ada#1
Ada
insert: database is read-only: insert is not allowed
update: database is read-only: update is not allowed
query: database is read-only: delete is not allowed
get_all: database is read-only: drop is not allowed
rows: 1
rows: 2
