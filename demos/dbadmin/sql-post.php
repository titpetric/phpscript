<?php

// @route POST /sql

include "bootstrap.php";
$query = trim($_POST["query"]);
if ($query == "") {
	die("Enter a SQL statement.");
}

$result = sql_execute($db, $query);
$message = $result["message"];
$rows = $result["rows"];
$result_columns = $result["result_columns"];
$title = "SQL console";

$tpl->load("sql.tpl");
$tpl->assign(array("title" => $title, "query" => $query, "message" => $message, "rows" => $rows, "result_columns" => $result_columns));
$tpl->render();
$db->close();
