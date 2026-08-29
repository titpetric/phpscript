<?php

// @route POST /t/{table}/empty
// @route POST /t/{table}/drop
// @route GET /table/create
// @route POST /table/create

include "bootstrap.php";

require_auth($ctx);

$where = path();
$creating = $where === "/table/create";

$opened = open_connection($ctx, $acl, $connections, $tpl, true);
$db = $opened["db"];
$driver = $opened["driver"];
$schema = $ctx["schema_name"];
$decision = $acl->decide($ctx);

if (!$creating) {
	require_csrf($ctx, $tpl);

	$table = (string)$_REQUEST["table"];
	$dropping = str_ends_with($where, "/drop");

	// The table name has to be typed into the form. A confirmation the user
	// can click through without reading is not a confirmation, and this is
	// the one action with nothing behind it to undo.
	if (post("confirmation", "") !== $table) {
		fail($ctx, $tpl, 400, "Type the table name exactly to confirm.");
	}

	try {
		if ($dropping) {
			$lost = $ddl->drop($db, $driver, $schema, $table, $decision["can_destroy"], $ctx);

			$sessions->set_flash($ctx, "Dropped " . $table . " and its " . (string)$lost . " row(s).");
			redirect_to("/tables");
		}

		$lost = $ddl->truncate($db, $driver, $schema, $table, $decision["can_destroy"], $ctx);

		$sessions->set_flash($ctx, "Emptied " . $table . "; " . (string)$lost . " row(s) removed.");
		redirect_to(table_url($table, ""));
	} catch (Exception $e) {
		fail($ctx, $tpl, 403, $e->getMessage());
	}
}

$errors = array();
$name = post("name", "");
$spec = array();
$types = ddl_dao::types($driver);

if (is_post()) {
	require_csrf($ctx, $tpl);

	$count = (int)post("columns", "0");
	for ($i = 0; $i < $count; $i += 1) {
		$field = post("name_" . (string)$i, "");
		if ($field === "") {
			continue;
		}

		$spec[] = array(
			"name" => $field,
			"type" => post("type_" . (string)$i, ddl_dao::default_type($driver)),
			"not_null" => checked("notnull_" . (string)$i),
			"primary" => checked("primary_" . (string)$i),
			"autoincrement" => checked("auto_" . (string)$i),
		);
	}

	try {
		$ddl->create($db, $driver, $schema, $name, $spec, $ctx);
		$sessions->set_flash($ctx, "Created table " . $name . ".");
		redirect_to(table_url($name, "/structure"));
	} catch (Exception $e) {
		$errors[] = $e->getMessage();
	}
}

// A blank row starts on the driver's string type, not on the first entry of
// types(). Somebody who fills in three column names and leaves the dropdowns
// alone should not get three integer columns.
$blank = ddl_dao::default_type($driver);

if (count($spec) == 0) {
	$spec = array(
		array(
			"name" => "id",
			"type" => $types[0],
			"not_null" => true,
			"primary" => false,
			"autoincrement" => true,
		),
		array(
			"name" => "",
			"type" => $blank,
			"not_null" => false,
			"primary" => false,
			"autoincrement" => false,
		),
		array(
			"name" => "",
			"type" => $blank,
			"not_null" => false,
			"primary" => false,
			"autoincrement" => false,
		),
		array(
			"name" => "",
			"type" => $blank,
			"not_null" => false,
			"primary" => false,
			"autoincrement" => false,
		),
	);
}

render($tpl, "Create table", "pane_create.tpl", $ctx, sidebar($ctx, $acl, $connections, $tables, ""), array(
	"errors" => $errors,
	"name" => $name,
	"spec" => $spec,
	"types" => $types,
	"decision" => $decision,
	"is_readonly" => false,
));
