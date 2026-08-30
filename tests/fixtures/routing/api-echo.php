<?php

// @route: /api/echo
// @route: /api/echo/{rest...}
//
// An httpbin-style echo of the request being served. Every value in the answer
// is read off the request and encoded here, in PHP: HTTP\Request::current()
// hands the request over, it does not render one.
//
// Two annotations because a trailing-segment route only matches once there is
// something to trail: /api/echo/{rest...} does not answer for /api/echo. Each
// is written without a method, which registers it for GET and for POST.

$request = HTTP\Request::current();

// The body is read the PHP way. $request->body is an io.ReadCloser, which is
// a stream a script has no way to hold, and only the two form content types
// reach $_POST; php://input is what a JSON API or a webhook reads.
$body = file_get_contents("php://input");

// The decoded body is present only when the client said it sent JSON, and is
// null otherwise, the way httpbin answers. header->get() is net/http's own
// case-insensitive lookup.
$json = null;
$type = $request->header->get("content-type");
if ($body !== "" && str_contains($type, "application/json")) {
	$json = json_decode($body, true);
}

// The trailing segments come from $_REQUEST rather than $request->pathvalue():
// each router publishes them under its own name, chi under "*", and $_REQUEST
// carries them under the name the annotation wrote.
$rest = isset($_REQUEST["rest"]) ? $_REQUEST["rest"] : "";

// $_GET, getallheaders() and $_POST are ordered arrays. The same three read off
// the request are Go maps -- $request->url->query() and $request->header --
// which re-randomise their order on every pass, so an answer built from them
// would differ between two identical requests. The cast to an object is what
// writes an empty one as {} rather than [].
$answer = array(
	"method" => $request->method,
	"proto" => $request->proto,
	"host" => $request->host,
	"path" => $request->url->path,
	"rest" => $rest,
	"url" => $_SERVER["REQUEST_SCHEME"] . "://" . $request->host . $request->requesturi,
	"args" => (object)$_GET,
	"headers" => (object)getallheaders(),
	"form" => (object)$_POST,
	"body" => $body,
	"json" => $json,
);

header("Content-Type: application/json");
echo json_encode($answer);
