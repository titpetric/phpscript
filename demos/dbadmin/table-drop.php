<?php

// @route POST /table/{table}/drop

include "bootstrap.php";
$table = $_PATH["table"];
table_info($db, $table);
if (!isset($_POST["confirmation"]) || $_POST["confirmation"] != $table) {
	die("Confirmation did not match the table name.");
}
$db->query("DROP TABLE " . qi($table));
redirect_to("/");
