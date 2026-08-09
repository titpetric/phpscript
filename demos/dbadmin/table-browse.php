<?php

// @route GET /table/{table}

include "bootstrap.php";
$table = $_PATH["table"];
$meta = table_info($db, $table);
$columns = columns_for($db, $table);
$without_rowid = strpos(strtoupper($meta["sql"]), "WITHOUT ROWID") !== false;
$page = 1;
if (isset($_GET["page"])) {
	$page = (int)$_GET["page"];
}
if ($page < 1) {
	$page = 1;
}
$search = "";
if (isset($_GET["q"])) {
	$search = trim($_GET["q"]);
}
$where = "";
$values = array();
if ($search != "" && count($columns) > 0) {
	$parts = array();
	foreach ($columns as $column) {
		$parts[] = "CAST(" . qi($column["name"]) . " AS TEXT) LIKE ?";
		$values[] = "%" . $search . "%";
	}
	$where = " WHERE " . implode(" OR ", $parts);
}
$count_args = array_merge(array("SELECT COUNT(*) AS total FROM " . qi($table) . $where), $values);
$count = call_user_func_array($db->get, $count_args);
$total = $count["total"] + 0;
$limit = 25;
$offset = $page - 1 * $limit;
$select = "SELECT ";
if (!$without_rowid) {
	$select .= "rowid AS _rowid_, ";
}
$select .= "* FROM " . qi($table) . $where . " LIMIT ? OFFSET ?";
$values[] = $limit;
$values[] = $offset;
$rows = call_user_func_array($db->get_all, array_merge(array($select), $values));
$title = "Browse " . $table;
include "templates/header.php";
include "templates/browse.php";
include "templates/footer.php";
$db->close();
