<?php

// @route GET /table/create

include "bootstrap.php";
$engine = "sqlite";
if (isset($_GET["engine"])) {
	$engine = strtolower($_GET["engine"]);
}

if (!in_array($engine, array("sqlite", "pgsql", "mysql"))) {
	$engine = "sqlite";
}

$title = "Create table";

render($tpl, "create", array("title" => $title, "engine" => $engine));
$db->close();
