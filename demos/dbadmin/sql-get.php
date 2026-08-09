<?php

// @route GET /sql

include "bootstrap.php";
$query = "SELECT * FROM catalogue LIMIT 50;";
$message = "";
$rows = array();
$result_columns = array();
$title = "SQL console";
include "templates/header.php";
include "templates/sql.php";
include "templates/footer.php";
$db->close();
