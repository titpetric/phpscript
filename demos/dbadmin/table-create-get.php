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

$tpl->load("create.tpl");
$tpl->assign(array("title" => $title, "engine" => $engine));
$tpl->render();
$db->close();
