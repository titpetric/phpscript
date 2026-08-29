<?php

// @route GET /admin/connection
// @route POST /admin/connection
// @route GET /admin/connection/test
// @route GET /admin/connection/{id}
// @route POST /admin/connection/{id}
// @route POST /admin/connection/{id}/delete

include "bootstrap.php";

require_admin($ctx, $tpl);

$where = path();
$panel = sidebar($ctx, $acl, $connections, $tables, "");

// The test page. Every connection is probed in turn and the result cached on
// the row, so the list page can show a status without doing this again.
if ($where === "/admin/connection/test") {
	$report = array();
	foreach ($connections->list_all() as $connection) {
		$result = $connections->test($connection, $tables);
		$report[] = array(
			"id" => $connection["id"],
			"name" => $connection["name"],
			"driver" => $connection["driver"],
			"dsn" => connection_dao::redact_dsn($connection["dsn"]),
			"status" => $result["status"],
			"message" => $result["message"],
			"tables" => $result["tables"],
			"columns" => $result["columns"],
			"schemas" => $result["schemas"],
		);
	}

	$audit->log($ctx, "admin", "connection", "", "tested every connection", array("connections" => count($report)));

	render($tpl, "Connection test", "pane_test.tpl", $ctx, $panel, array("report" => $report));
	exit();
}

$errors = array();

if ($where === "/admin/connection") {
	if (is_post()) {
		require_csrf($ctx, $tpl);

		try {
			$id = $connections->create($ctx, post("name", ""), post("dsn", ""), post("schema", ""), checked("readonly"));

			$sessions->set_flash($ctx, "Connection created. Test it to check it can be reached.");
			redirect_to("/admin/connection/" . (string)$id);
		} catch (Exception $e) {
			$errors[] = $e->getMessage();
		}
	}

	$listing = array();
	foreach ($connections->list_all() as $connection) {
		$row = array_copy($connection);
		$row["dsn"] = connection_dao::redact_dsn($connection["dsn"]);
		$listing[] = $row;
	}

	render($tpl, "Connections", "pane_connections.tpl", $ctx, $panel, array(
		"connections" => $listing,
		"errors" => $errors,
		"name" => post("name", ""),
		"dsn" => post("dsn", ""),
	));
	exit();
}

$id = (int)$_REQUEST["id"];
$connection = $connections->find($id);
if (!$connection) {
	fail($ctx, $tpl, 404, "No such connection.");
}

if (str_ends_with($where, "/delete")) {
	require_csrf($ctx, $tpl);

	if (post("confirmation", "") !== $connection["name"]) {
		fail($ctx, $tpl, 400, "Type the connection name exactly to confirm.");
	}

	$connections->remove($ctx, $id, $groups, $sessions);
	$sessions->set_flash($ctx, "Connection " . $connection["name"] . " deleted.");
	redirect_to("/admin/connection");
}

if (is_post()) {
	require_csrf($ctx, $tpl);

	try {
		$connections->update($ctx, $id, post("dsn", ""), post("schema", ""), checked("enabled"), checked("readonly"));
		$sessions->set_flash($ctx, "Connection updated.");
		redirect_to("/admin/connection/" . (string)$id);
	} catch (Exception $e) {
		$errors[] = $e->getMessage();
	}

	$connection = $connections->find($id);
}

$result = null;
if (query("test", "") !== "") {
	$result = $connections->test($connection, $tables);
	$connection = $connections->find($id);
}

render($tpl, "Connection: " . $connection["name"], "pane_connection.tpl", $ctx, $panel, array(
	"connection" => $connection,
	"redacted" => connection_dao::redact_dsn($connection["dsn"]),
	"errors" => $errors,
	"result" => $result,
	"grants" => $connections->grants($id),
));
