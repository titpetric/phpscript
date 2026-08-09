<?php

include "include/Database.php";
$db = new Database;
$db->connect("dbadmin");
$catalogue = $db->get("SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'catalogue'");
if (!$catalogue) {
	$db->query("CREATE TABLE catalogue (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, category TEXT NOT NULL DEFAULT 'General', notes TEXT, created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)");
	$db->query("INSERT INTO catalogue (name, category, notes) VALUES (?, ?, ?)", "SQLite Handbook", "Books", "A sample record you can edit or export.");
	$db->query("INSERT INTO catalogue (name, category, notes) VALUES (?, ?, ?)", "Desk Lamp", "Equipment", "Created automatically on the first request.");
	$db->query("INSERT INTO catalogue (name, category, notes) VALUES (?, ?, ?)", "Local database", "Projects", "Explore this demo using the navigation above.");
}

function h($value) {
	if ($value === null) {
		return "<span class=\"null\">NULL</span>";
	}
	return htmlspecialchars("" . $value);
}

function identifier_ok($name) {
	return preg_match("/^[A-Za-z_][A-Za-z0-9_]*$/", $name) == 1;
}

function qi($name) {
	if (!identifier_ok($name)) {
		die("Invalid SQL identifier.");
	}
	return "\"" . $name . "\"";
}

function table_info($db, $table) {
	if (!identifier_ok($table)) {
		die("Invalid table name.");
	}
	$found = $db->get("SELECT name, sql FROM sqlite_master WHERE type = 'table' AND name = ? AND name NOT LIKE 'sqlite_%'", $table);
	if (!$found) {
		die("Table not found.");
	}
	return $found;
}

function columns_for($db, $table) {
	table_info($db, $table);
	return $db->get_all("PRAGMA table_info(" . qi($table) . ")");
}

function redirect_to($url) {
	header("Location: " . $url);
	exit();
}

function csv_cell($value) {
	if ($value === null) {
		$value = "";
	}
	return "\"" . str_replace("\"", "\"\"", "" . $value) . "\"";
}
