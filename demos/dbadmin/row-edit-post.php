<?php

// @route POST /table/{table}/row/{rowid}/edit

include "bootstrap.php";
$table = $_REQUEST["table"];
$meta = table_info($db, $table);
if (strpos(strtoupper($meta["sql"]), "WITHOUT ROWID") !== false) {
	die("This table has no rowid.");
}

$rowid = (int)$_REQUEST["rowid"];
$columns = columns_for($db, $table);
$sets = array();
$values = array();
foreach ($columns as $column) {
	$sets[] = qi($column["name"]) . " = ?";
	$cid = $column["cid"];
	if (isset($_POST["null_" . $cid])) {
		$values[] = null;
	} else {
		$values[] = $_POST["value_" . $cid];
	}
}

$values[] = $rowid;

call_user_func_array($db->query, array_merge(array("UPDATE " . qi($table) . " SET " . implode(",", $sets) . " WHERE rowid = ?"), $values));
redirect_to("/table/" . $table);
