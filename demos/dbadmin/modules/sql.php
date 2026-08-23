<?php

// @route GET /sql
// @route POST /sql

include "bootstrap.php";

require_auth($ctx);

// The console is opened for writing when the grant allows it, so that a write
// statement is refused by the grant rather than by the client with a message
// about the verb.
$grant = $acl->may_use($ctx, $ctx["connection_id"]);
$opened = open_connection($ctx, $acl, $connections, $tpl, false);

if (!$grant["is_readonly"]) {
	$opened = open_connection($ctx, $acl, $connections, $tpl, true);
}

$decision = $acl->decide($ctx);
$statement = post("statement", "");
$result = array(
	"message" => "",
	"rows" => array(),
	"columns" => array(),
	"truncated" => false,
);
$errors = array();

if (is_post()) {
	require_csrf($ctx, $tpl);

	try {
		$result = $console->run($opened["db"], $opened["driver"], $statement, $decision["can_destroy"], $grant["is_readonly"], $ctx);
	} catch (Exception $e) {
		$errors[] = $e->getMessage();
	}
}

render($tpl, "SQL console", "pane_sql.tpl", $ctx, sidebar($ctx, $acl, $connections, $tables, ""), array(
	"statement" => $statement,
	"result" => $result,
	"errors" => $errors,
	"kind" => $statement === "" ? "" : sql_dao::classify($statement),
	"decision" => $decision,
	"is_readonly" => $grant["is_readonly"],
));
