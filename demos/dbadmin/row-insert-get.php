<?php

// @route GET /table/{table}/insert

include "bootstrap.php";
$table = $_PATH["table"];
$columns = columns_for($db, $table);
$mode = "Insert";
$title = "Insert · " . $table;

render($tpl, "row-form", array("title" => $title, "table" => $table, "columns" => $columns, "mode" => $mode, "row" => array()));
$db->close();
