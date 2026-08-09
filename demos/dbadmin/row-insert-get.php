<?php

// @route GET /table/{table}/insert

include "bootstrap.php";
$table = $_PATH["table"];
$columns = columns_for($db, $table);
$mode = "Insert";
$title = "Insert · " . $table;
include "templates/header.php";
include "templates/row-form.php";
include "templates/footer.php";
$db->close();
