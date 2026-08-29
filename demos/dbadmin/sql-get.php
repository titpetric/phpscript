<?php

// @route GET /sql

include "bootstrap.php";
$query = "";
if (isset($_GET["query"])) {
	$query = trim($_GET["query"]);
}

// The console opens on an example statement, so the page shows what a result
// looks like before anything is typed into it.
if ($query == "") {
	$query = "SELECT * FROM catalogue LIMIT 50;";
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
