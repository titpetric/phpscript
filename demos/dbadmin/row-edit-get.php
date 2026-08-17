<?php

// @route GET /table/{table}/row/{rowid}/edit

include "bootstrap.php";
$table = $_PATH["table"];
$meta = table_info($db, $table);
if (strpos(strtoupper($meta["sql"]), "WITHOUT ROWID") !== false) {
	die("This table has no rowid and cannot be edited here.");
}

$rowid = (int)$_PATH["rowid"];
$columns = columns_for($db, $table);
$row = $db->get("SELECT * FROM " . qi($table) . " WHERE rowid = ?", $rowid);
if (!$row) {
	die("Row not found.");
}

$mode = "Update";
$title = "Edit row · " . $table;

$tpl->load("row-form.tpl");
$tpl->assign(array("title" => $title, "table" => $table, "columns" => $columns, "mode" => $mode, "row" => $row));
$tpl->render();
$db->close();
