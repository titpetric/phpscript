<?php

// @route GET /table/{table}/structure

include "bootstrap.php";
$table = $_PATH["table"];
$meta = table_info($db, $table);
$columns = columns_for($db, $table);
$indexes = $db->get_all("PRAGMA index_list(" . qi($table) . ")");
$title = "Structure · " . $table;
include "templates/header.php";
include "templates/structure.php";
include "templates/footer.php";
$db->close();
