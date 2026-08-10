<?php

// @route GET /table/{table}/structure

include "bootstrap.php";
$table = $_PATH["table"];
$meta = table_info($db, $table);
$columns = columns_for($db, $table);
$indexes = $db->get_all("PRAGMA index_list(" . qi($table) . ")");
$title = "Structure · " . $table;
render($tpl, "structure", array("title" => $title, "table" => $table, "meta" => $meta, "columns" => $columns, "indexes" => $indexes));
$db->close();
