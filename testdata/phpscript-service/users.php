<?php

// @route GET /users/{id}

// {id} arrives in $_REQUEST under the name the annotation declares, which is
// what $r->path_value("id") answers on the bridge.
//
// REQUEST_URI carries the query string with it, so the path is what is left of
// the first question mark. json_encode writes the document the bridge's
// JSON\Encoder writes, and the newline after it is the encoder's.

header("Content-Type: application/json");

$path = explode("?", $_SERVER["REQUEST_URI"]);

echo json_encode(array("id" => $_REQUEST["id"], "path" => $path[0])) . "\n";
