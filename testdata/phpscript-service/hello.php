<?php

// @route GET /hello

// $_GET is the query, decoded before the endpoint runs. The bridge reads the
// same value through $r->url->query()->get("name"), which answers "" for a
// name that is not there; isset() is what does that here.

header("Content-Type: text/plain; charset=utf-8");

$name = isset($_GET["name"]) ? $_GET["name"] : "";
if ($name === "") {
	$name = "world";
}

echo "hello " . $name . "\n";
