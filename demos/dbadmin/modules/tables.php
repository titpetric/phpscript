<?php

// @route GET /tables

include "bootstrap.php";

require_auth($ctx);

$opened = open_connection($ctx, $acl, $connections, $tpl, false);
$listing = array();
$error = "";
$counted = true;

try {
	$listing = $tables->tables($opened["db"], $opened["driver"], $ctx["schema_name"]);

	// sqlite reports no row estimate, so the listing arrives with zeroes and
	// the counts are taken one at a time, up to a cap.
	if ($opened["driver"] === "sqlite") {
		$filled = $tables->fill_row_counts($opened["db"], $opened["driver"], $ctx["schema_name"], $listing, 50);
		$listing = $filled["rows"];
		$counted = $filled["counted"];
	}
} catch (Exception $e) {
	$error = $e->getMessage();
}

// The right panel is the list with its actions; the left is the same tables as
// a menu. Both come from one query, because they are one answer.
$panel = sidebar_off();
$panel["connections"] = $acl->connections_for($ctx);
$panel["tables"] = $listing;
$panel["error"] = $error;
if ($opened["driver"] !== "sqlite") {
	$panel["schemas"] = $tables->schemas($opened["db"], $opened["driver"]);
}

render($tpl, $ctx["connection_name"], "pane_tables.tpl", $ctx, $panel, array(
	"tables" => $listing,
	"error" => $error,
	"decision" => $acl->decide($ctx),
	"is_readonly" => $opened["is_readonly"],
	"exact_rows" => $opened["driver"] === "sqlite" && $counted,
	"counted" => $counted,
));
