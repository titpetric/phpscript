<?php

// @route GET /

include "bootstrap.php";

require_auth($ctx);

// The landing page is the connection chooser. A session that already has a
// connection goes straight to its tables, so the chooser is the page you see
// when there is a choice left to make.
if ($ctx["connection_id"] > 0) {
	redirect_to("/tables");
}

$available = $acl->connections_for($ctx);

render($tpl, "Connections", "pane_choose.tpl", $ctx, sidebar($ctx, $acl, $connections, $tables, ""), array("connections" => $available, "decision" => $acl->decide($ctx)));
