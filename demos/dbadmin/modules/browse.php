<?php

// @route GET /t/{table}
// @route GET /t/{table}/structure
// @route GET /t/{table}/export

include "bootstrap.php";

require_auth($ctx);

$table = (string)$_REQUEST["table"];
$opened = open_connection($ctx, $acl, $connections, $tpl, false);
$db = $opened["db"];
$driver = $opened["driver"];
$schema = $ctx["schema_name"];

if (!$tables->exists($db, $driver, $schema, $table)) {
	fail($ctx, $tpl, 404, "No table named " . $table . " on this connection.");
}

$panel = sidebar($ctx, $acl, $connections, $tables, $table);
$decision = $acl->decide($ctx);
$where = path();

if (str_ends_with($where, "/export")) {
	// A download is not a page: the headers are staged before anything is
	// written, and the body is the file.
	header("Content-Type: text/csv; charset=utf-8");
	header("Content-Disposition: attachment; filename=\"" . $table . ".csv\"");
	$browse->export($db, $driver, $schema, $table, $ctx);
	exit();
}

if (str_ends_with($where, "/structure")) {
	render($tpl, $table . " structure", "pane_structure.tpl", $ctx, $panel, array(
		"table" => $table,
		"columns" => $tables->columns($db, $driver, $schema, $table),
		"indexes" => $tables->indexes($db, $driver, $schema, $table),
		"identity" => $tables->identity($db, $driver, $schema, $table),
		"definition" => $tables->definition($db, $driver, $table),
		"rows" => $tables->row_count($db, $driver, $schema, $table),
		"decision" => $decision,
		"is_readonly" => $opened["is_readonly"],
		"tab" => "structure",
	));
	exit();
}

$page = (int)query("page", "1");
if ($page < 1) {
	$page = 1;
}

$result = $browse->page($db, $driver, $schema, $table, query("q", ""), $page);

render($tpl, $table, "pane_browse.tpl", $ctx, $panel, array(
	"table" => $table,
	"result" => $result,
	"decision" => $decision,
	"is_readonly" => $opened["is_readonly"],
	"tab" => "browse",
));
