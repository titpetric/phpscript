name: database bridge
description: Database.php can use the Go DatabaseDriver with sqlite.
---
<?php

include("code/Database.php");

$db = new Database();
$db->connect("sqlite_test");

$db->query("drop table if exists users");
$db->query("create table users (id integer primary key autoincrement, name text)");

$db->insert("users", array("name" => "Ada"));
$db->insert("users", array("name" => "Grace"));

$row = $db->get("select id, name from users where name = ?", "Ada");

echo $row["name"] . "#" . $row["id"] . "\n";

$users = $db->get_all("select name from users order by id");
foreach ($users as $row) {
	echo $row["name"] . "\n";
}
---
Ada#1
Ada
Grace
