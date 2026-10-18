<?php

// @route GET /users/{id}

// {id} arrives in $_REQUEST under the name the annotation declares.
// REQUEST_URI carries the query string with it, so the path is what stands left
// of the first question mark.

header("Content-Type: application/json");

$path = explode("?", $_SERVER["REQUEST_URI"]);

echo json_encode(array("id" => $_REQUEST["id"], "path" => $path[0])) . "\n";
