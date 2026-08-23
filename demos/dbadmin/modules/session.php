<?php

// @route POST /session/connection
// @route POST /session/schema
// @route POST /session/destructive

include "bootstrap.php";

require_auth($ctx);
require_csrf($ctx, $tpl);

$where = path();
$back = safe_path(post("back", "/tables"), "/tables");

if ($where === "/session/connection") {
	$connection_id = (int)post("connection_id", "0");

	if ($connection_id == 0) {
		$sessions->set_connection($ctx, 0, "");
		redirect_to("/");
	}

	$grant = $acl->may_use($ctx, $connection_id);
	if (!$grant["allowed"]) {
		fail($ctx, $tpl, 403, "You do not have access to that connection.");
	}

	$connection = $connections->find($connection_id);

	$sessions->set_connection($ctx, $connection_id, (string)$connection["default_schema"]);
	redirect_to("/tables");
}

if ($where === "/session/schema") {
	if ($ctx["connection_id"] == 0) {
		redirect_to("/");
	}

	// The schema is a name from the target server, not an identifier this
	// application chose, so it is checked against what the server reports
	// rather than against a pattern.
	$opened = open_connection($ctx, $acl, $connections, $tpl, false);
	$schema = post("schema", "");
	if (!in_array($schema, $tables->schemas($opened["db"], $opened["driver"]))) {
		fail($ctx, $tpl, 400, "No such schema on this connection.");
	}

	$sessions->set_schema($ctx, $schema);
	redirect_to("/tables");
}

// Destructive mode. The account policy decides whether the switch exists at
// all; a request that arrives without the switch having been drawn is refused
// and recorded, because it did not come from the page.
$decision = $acl->decide($ctx);
if (!$decision["offers_toggle"]) {
	$audit->log($ctx, "denied", "user_session", $ctx["session_id"], "attempted to toggle destructive mode", array("policy" => $decision["policy"]));
	fail($ctx, $tpl, 403, "Destructive mode is not available for this account.");
}

$sessions->set_destructive($ctx, post("on", "") !== "");
redirect_to($back);
