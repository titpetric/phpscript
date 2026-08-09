<?php

// @route GET /

include "bootstrap.php";
$raw = $db->get_all("SELECT name, sql FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name");
$tables = array();
$column_count = 0;
$row_count = 0;
foreach ($raw as $table) {
	$name = $table["name"];
	$columns = columns_for($db, $name);
	$count = $db->get("SELECT COUNT(*) AS total FROM " . qi($name));
	$table["columns"] = count($columns);
	$table["rows"] = $count["total"] + 0;
	$column_count = $column_count + count($columns);
	$row_count = $row_count + $table["rows"];
	$tables[] = $table;
}
$table_count = count($tables);
$title = "Database overview";
include "templates/header.php";
include "templates/dashboard.php";
include "templates/footer.php";
$db->close();
