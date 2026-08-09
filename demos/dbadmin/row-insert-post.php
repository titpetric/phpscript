<?php

// @route POST /table/{table}/insert

include "bootstrap.php";
$table = $_PATH["table"];
$columns = columns_for($db, $table);
$names = array();
$marks = array();
$values = array();
foreach ($columns as $column) {
	$names[] = qi($column["name"]);
	$marks[] = "?";
	$cid = $column["cid"];
	if (isset($_POST["null_" . $cid])) {
		$values[] = null;
	} else {
		$values[] = $_POST["value_" . $cid];
	}
}
$args = array_merge(array("INSERT INTO " . qi($table) . " (" . implode(",", $names) . ") VALUES (" . implode(",", $marks) . ")"), $values);
call_user_func_array($db->query, $args);
redirect_to("/table/" . $table);
