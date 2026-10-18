<?php

// @route POST /echo

// $_POST is the form body, parsed before the endpoint runs, and $_GET is the
// query. The body comes first and the query is the fallback.

header("Content-Type: application/json");

$path = explode("?", $_SERVER["REQUEST_URI"]);

$name = "";
if (isset($_POST["name"])) {
	$name = $_POST["name"];
} else if (isset($_GET["name"])) {
	$name = $_GET["name"];
}

echo json_encode(array(
	"method" => $_SERVER["REQUEST_METHOD"],
	"path" => $path[0],
	"query" => $_SERVER["QUERY_STRING"],
	"user_agent" => $_SERVER["HTTP_USER_AGENT"],
	"name" => $name,
)) . "\n";
