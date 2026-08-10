<?php

// @route GET /table/{table}/export

include "bootstrap.php";
$table = $_PATH["table"];
$columns = columns_for($db, $table);
$rows = $db->get_all("SELECT * FROM " . qi($table));

header("Content-Type: text/csv; charset=utf-8");
header("Content-Disposition: attachment; filename=\"" . $table . ".csv\"");
$line = array();
foreach ($columns as $column) {
	$line[] = csv_cell($column["name"]);
}

echo implode(",", $line) . "\r\n";
foreach ($rows as $row) {
	$line = array();
	foreach ($columns as $column) {
		$line[] = csv_cell($row[$column["name"]]);
	}

	echo implode(",", $line) . "\r\n";
}

$db->close();
