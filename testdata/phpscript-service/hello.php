<?php

// @route GET /hello

// $_GET is the query, decoded before the endpoint runs.

header("Content-Type: text/plain; charset=utf-8");

$name = isset($_GET["name"]) ? $_GET["name"] : "";
if ($name === "") {
	$name = "world";
}

echo "hello " . $name . "\n";
