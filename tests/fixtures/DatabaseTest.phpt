name: database bridge
description: Database.php can use the Go DatabaseDriver with sqlite.
---
<?php

class PDO { const FETCH_ASSOC = 2; }

include("code/Database.php");

$db = new Database();
$db->connect("sqlite://file:phpscript-test?mode=memory&cache=shared");
$db->query("create table users (id integer primary key autoincrement, name text)");
$db->insert("users", array("name" => "Ada"));
$db->insert("users", array("name" => "Grace"));

$stmt = $db->query("select id, name from users where name = ?", "Ada");
$row = $db->fetch($stmt);
echo $row["name"] . "#" . $row["id"] . "\n";
$stmt->closeCursor();

$stmt = $db->query("select name from users order by id");
while ($row = $db->fetch($stmt)) {
	echo $row["name"] . "\n";
}
$stmt->closeCursor();
---
Ada#1
Ada
Grace
