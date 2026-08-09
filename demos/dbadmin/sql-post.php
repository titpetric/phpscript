<?php

// @route POST /sql

include "bootstrap.php";
$query = trim($_POST["query"]);
if ($query == "") {
	die("Enter a SQL statement.");
}
$statement = $db->query($query);
$prefix = strtolower(ltrim($query));
$rows = array();
$result_columns = array();
$message = "Statement executed successfully.";
if (strpos($prefix, "select") === 0 || strpos($prefix, "pragma") === 0 || strpos($prefix, "with") === 0 || strpos($prefix, "explain") === 0) {
	$row = $statement->fetch();
	while ($row) {
		if (count($result_columns) == 0) {
			$result_columns = array_keys($row);
		}
		$rows[] = $row;
		$row = $statement->fetch();
	}
	$message = "Query completed; " . count($rows) . " row(s) returned.";
}
$statement->close();
$title = "SQL console";
include "templates/header.php";
include "templates/sql.php";
include "templates/footer.php";
$db->close();
