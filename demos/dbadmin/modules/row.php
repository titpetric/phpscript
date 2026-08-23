<?php

// @route GET /t/{table}/insert
// @route POST /t/{table}/insert
// @route GET /t/{table}/row/{key}/edit
// @route POST /t/{table}/row/{key}/edit
// @route POST /t/{table}/row/{key}/delete

include "bootstrap.php";

require_auth($ctx);

$table = (string)$_PATH["table"];
$where = path();
$deleting = str_ends_with($where, "/delete");
$inserting = str_ends_with($where, "/insert");

// Every route here writes, so the connection is opened for writing and a
// read-only grant is refused before anything else happens.
$opened = open_connection($ctx, $acl, $connections, $tpl, true);
$db = $opened["db"];
$driver = $opened["driver"];
$schema = $ctx["schema_name"];

if (!$tables->exists($db, $driver, $schema, $table)) {
	fail($ctx, $tpl, 404, "No table named " . $table . " on this connection.");
}

$decision = $acl->decide($ctx);
$key = isset($_PATH["key"]) ? (string)$_PATH["key"] : "";

if ($deleting) {
	require_csrf($ctx, $tpl);

	try {
		$rows->delete($db, $driver, $schema, $table, $key, $decision["can_destroy"], $ctx);
		$sessions->set_flash($ctx, "Row deleted from " . $table . ".");
	} catch (Exception $e) {
		fail($ctx, $tpl, 403, $e->getMessage());
	}

	redirect_to(table_url($table, ""));
}

$errors = array();
$columns = $tables->columns($db, $driver, $schema, $table);
$identity = $tables->identity($db, $driver, $schema, $table);

if (!$inserting && $identity["kind"] === "none") {
	fail($ctx, $tpl, 409, "This table has no primary key, so a single row cannot be addressed. Browse and insert still work.");
}

// The posted form is the source of truth on a failed submit, so a mistake is
// corrected rather than retyped.
$values = $inserting ? row_dao::blank($columns) : array();
$nulls = array();

if (is_post()) {
	require_csrf($ctx, $tpl);

	$input = array();
	foreach ($columns as $column) {
		$name = $column["name"];
		if (isset($_POST["f_" . $name])) {
			$input[$name] = (string)$_POST["f_" . $name];
		}

		if (isset($_POST["null_" . $name])) {
			$nulls[$name] = true;
		}
	}

	$values = $input;

	try {
		if ($inserting) {
			$rows->insert($db, $driver, $schema, $table, $input, $nulls, $ctx);
			$sessions->set_flash($ctx, "Row inserted into " . $table . ".");
		} else {
			$rows->update($db, $driver, $schema, $table, $key, $input, $nulls, $ctx);
			$sessions->set_flash($ctx, "Row updated in " . $table . ".");
		}

		redirect_to(table_url($table, ""));
	} catch (Exception $e) {
		$errors[] = $e->getMessage();
	}
}

if (!$inserting && count($values) == 0) {
	$found = $browse->find($db, $driver, $schema, $table, $key);
	$values = $found["row"];
}

// The form indexes both maps by column name for every column, so a template
// never asks for a key that was not set. A missing key is a runtime error
// here, not an empty string.
$fields = array();
$flags = array();
foreach ($columns as $column) {
	$name = $column["name"];
	$fields[$name] = array_key_exists($name, $values) ? $values[$name] : "";
	$flags[$name] = array_key_exists($name, $nulls);
}

render($tpl, $inserting ? ("Insert into " . $table) : ("Edit row in " . $table), "pane_row.tpl", $ctx, sidebar($ctx, $acl, $connections, $tables, $table), array(
	"table" => $table,
	"columns" => $columns,
	"identity" => $identity,
	"values" => $fields,
	"nulls" => $flags,
	"errors" => $errors,
	"inserting" => $inserting,
	"key" => $key,
	"decision" => $decision,
	"is_readonly" => false,
	"tab" => $inserting ? "insert" : "",
));
