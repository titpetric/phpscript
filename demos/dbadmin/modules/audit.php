<?php

// @route GET /admin/audit

include "bootstrap.php";

require_admin($ctx, $tpl);

$page = (int)query("page", "1");
if ($page < 1) {
	$page = 1;
}

$size = 50;
$filter = array(
	"user_id" => (int)query("user", "0"),
	"connection_id" => (int)query("connection", "0"),
	"action" => query("action", ""),
	"rel_table" => query("table", ""),
);

$result = $audit->page($filter, ($page - 1) * $size, $size);
$pages = max2(1, div_ceil($result["total"], $size));

render($tpl, "Audit log", "pane_audit.tpl", $ctx, sidebar($ctx, $acl, $connections, $tables, ""), array(
	"log" => $result["rows"],
	"total" => $result["total"],
	"page" => $page,
	"pages" => $pages,
	"filter" => $filter,
	"actions" => audit_dao::actions(),
	"actors" => $audit->actors(),
));
