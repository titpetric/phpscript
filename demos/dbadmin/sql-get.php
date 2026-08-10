<?php

// @route GET /sql

include "bootstrap.php";
$query = "SELECT * FROM catalogue LIMIT 50;";
$message = "";
$rows = array();
$result_columns = array();
$title = "SQL console";
render($tpl, "sql", array("title" => $title, "query" => $query, "message" => $message, "rows" => $rows, "result_columns" => $result_columns));
$db->close();
