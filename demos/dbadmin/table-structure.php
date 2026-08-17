<?php

// @route GET /table/{table}/structure

include "bootstrap.php";
$table = $_PATH["table"];
$meta = table_info($db, $table);
$columns = columns_for($db, $table);
$indexes = $db->get_all("PRAGMA index_list(" . qi($table) . ")");
$title = "Structure · " . $table;

$tpl->load("structure.tpl");
$tpl->assign(array("title" => $title, "table" => $table, "meta" => $meta, "columns" => $columns, "indexes" => $indexes));
$tpl->render();
$db->close();
