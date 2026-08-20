<?php
require "vendor/autoload.php";

$request_start_time = microtime(true);

$db = new Database("dbadmin");
$tpl = new MiniTPL\Template;

function format_memory_bytes($bytes) {
	if ($bytes >= 1048576) {
		return sprintf("%.2f MB", $bytes / 1048576);
	}
	if ($bytes >= 1024) {
		return sprintf("%.2f KB", $bytes / 1024);
	}
	return $bytes . " B";
}

function render_footer_stats() {
	global $request_start_time;
	$duration = microtime(true) - $request_start_time;
	$memory = memory_get_usage();
	return sprintf("page rendered in %.4f seconds, used %s memory", $duration, format_memory_bytes($memory));
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

/**
 * Runs a console statement and returns the message, rows and column names.
 *
 * query() reports success as a boolean, so statements that return rows are
 * read with get_all() instead. Rows arrive as maps without a column order;
 * the names are sorted to keep the rendered table stable between requests.
 */
function sql_execute($db, $query) {
	$prefix = strtolower(ltrim($query));
	$returns_rows = strpos($prefix, "select") === 0 || strpos($prefix, "pragma") === 0 || strpos($prefix, "with") === 0 || strpos($prefix, "explain") === 0;
	if (!$returns_rows) {
		$db->query($query);
		return array("message" => "Statement executed successfully.", "rows" => array(), "result_columns" => array());
	}

	$rows = $db->get_all($query);
	$result_columns = array();
	if (count($rows) > 0) {
		$result_columns = array_keys($rows[0]);
		sort($result_columns);
	}

	return array("message" => "Query completed; " . count($rows) . " row(s) returned.", "rows" => $rows, "result_columns" => $result_columns);
}
