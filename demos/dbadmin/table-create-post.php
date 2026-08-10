<?php

// @route POST /table/create

include "bootstrap.php";
$table = $_POST["table_name"];
if (!identifier_ok($table)) {
	die("Invalid table name.");
}

$existing = $db->get("SELECT name FROM sqlite_master WHERE name = ?", $table);
if ($existing) {
	die("A table with that name already exists.");
}

$engine = "sqlite";
if (isset($_POST["engine"])) {
	$engine = strtolower($_POST["engine"]);
}

$sqlite_types = array("INTEGER", "TEXT", "REAL", "NUMERIC", "BLOB", "BOOLEAN", "DATE", "TIME", "DATETIME", "TIMESTAMP");
$pgsql_types = array("SMALLINT", "INTEGER", "BIGINT", "SERIAL", "BIGSERIAL", "NUMERIC", "REAL", "DOUBLE PRECISION", "BOOLEAN", "CHAR", "VARCHAR", "TEXT", "DATE", "TIME", "TIMESTAMP", "TIMESTAMPTZ", "JSON", "JSONB", "BYTEA", "UUID");
$mysql_types = array("TINYINT", "SMALLINT", "MEDIUMINT", "INT", "BIGINT", "DECIMAL", "FLOAT", "DOUBLE", "BOOLEAN", "CHAR", "VARCHAR", "TEXT", "MEDIUMTEXT", "LONGTEXT", "DATE", "TIME", "DATETIME", "TIMESTAMP", "YEAR", "JSON", "BLOB");
$allowed_types = $sqlite_types;
if ($engine == "pgsql") {
	$allowed_types = $pgsql_types;
} elseif ($engine == "mysql") {
	$allowed_types = $mysql_types;
} elseif ($engine != "sqlite") {
	die("Invalid database type.");
}

$column_count = 0;
if (isset($_POST["column_count"])) {
	$column_count = (int)$_POST["column_count"];
}

if ($column_count < 1 || $column_count > 100) {
	die("Create between 1 and 100 columns.");
}

$definitions = array();
for ($i = 1; $i <= $column_count; $i++) {
	$key = "name_" . $i;
	$name = "";
	if (isset($_POST[$key])) {
		$name = trim($_POST[$key]);
	}

	if ($name != "") {
		if (!identifier_ok($name)) {
			die("Invalid column name.");
		}

		$type = strtoupper($_POST["type_" . $i]);
		if (!in_array($type, $allowed_types)) {
			die("Invalid column type for " . $engine . ".");
		}

		$definition = qi($name) . " " . $type;
		if (isset($_POST["notnull_" . $i])) {
			$definition .= " NOT NULL";
		}

		$default = trim($_POST["default_" . $i]);
		if ($default != "") {
			if (!preg_match("/^(NULL|CURRENT_TIMESTAMP|-?[0-9]+|\\x27[^\\x27]*\\x27)$/", $default)) {
				die("Defaults may be NULL, CURRENT_TIMESTAMP, a number, or a single-quoted string.");
			}

			$definition .= " DEFAULT " . $default;
		}

		if (isset($_POST["pk_" . $i])) {
			$definition .= " PRIMARY KEY";
		}

		$definitions[] = $definition;
	}
}

if (count($definitions) == 0) {
	die("At least one column is required.");
}

$db->query("CREATE TABLE " . qi($table) . " (" . implode(", ", $definitions) . ")");
redirect_to("/table/" . $table . "/structure");
