<?php

// @route POST /table/{table}/row/{rowid}/delete

include "bootstrap.php";
$table = $_REQUEST["table"];
$meta = table_info($db, $table);
if (strpos(strtoupper($meta["sql"]), "WITHOUT ROWID") !== false) {
	die("This table has no rowid.");
}

$rowid = (int)$_REQUEST["rowid"];

$db->query("DELETE FROM " . qi($table) . " WHERE rowid = ?", $rowid);
redirect_to("/table/" . $table);
