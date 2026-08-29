<?php

// @route GET /admin/group
// @route POST /admin/group
// @route GET /admin/group/{id}
// @route POST /admin/group/{id}
// @route POST /admin/group/{id}/delete

include "bootstrap.php";

require_admin($ctx, $tpl);

$where = path();
$panel = sidebar($ctx, $acl, $connections, $tables, "");
$errors = array();

if ($where === "/admin/group") {
	if (is_post()) {
		require_csrf($ctx, $tpl);

		try {
			$id = $groups->create($ctx, post("name", ""), post("description", ""), post("policy", "allowed"));

			$sessions->set_flash($ctx, "Group created. Grant it a connection to make it useful.");
			redirect_to("/admin/group/" . (string)$id);
		} catch (Exception $e) {
			$errors[] = $e->getMessage();
		}
	}

	render($tpl, "Groups", "pane_groups.tpl", $ctx, $panel, array(
		"groups" => $groups->list_all(),
		"policies" => user_dao::policies(),
		"errors" => $errors,
		"name" => post("name", ""),
	));
	exit();
}

$id = (int)$_REQUEST["id"];
$group = $groups->find($id);
if (!$group) {
	fail($ctx, $tpl, 404, "No such group.");
}

if (str_ends_with($where, "/delete")) {
	require_csrf($ctx, $tpl);

	$groups->remove($ctx, $id);
	$sessions->set_flash($ctx, "Group " . $group["name"] . " deleted.");
	redirect_to("/admin/group");
}

if (is_post()) {
	require_csrf($ctx, $tpl);

	$action = post("action", "profile");

	try {
		if ($action === "members") {
			$chosen = array();
			foreach ($users->list_all() as $user) {
				if (checked("user_" . (string)$user["id"])) {
					$chosen[] = (int)$user["id"];
				}
			}

			$groups->set_members($ctx, $id, $chosen);
			$sessions->set_flash($ctx, "Members updated.");
		} elseif ($action === "grants") {
			// A connection absent from the map is not granted; present with
			// readonly set is granted for reading only. The two are
			// different answers and the form sends both.
			$chosen = array();
			foreach ($connections->list_all() as $connection) {
				if (checked("grant_" . (string)$connection["id"])) {
					$chosen[(int)$connection["id"]] = checked("ro_" . (string)$connection["id"]);
				}
			}

			$groups->set_grants($ctx, $id, $chosen);
			$sessions->set_flash($ctx, "Connections updated.");
		} else {
			$groups->update($ctx, $id, post("name", ""), post("description", ""), post("policy", "allowed"));
			$sessions->set_flash($ctx, "Group updated.");
		}

		redirect_to("/admin/group/" . (string)$id);
	} catch (Exception $e) {
		$errors[] = $e->getMessage();
	}

	$group = $groups->find($id);
}

$member_ids = array();
foreach ($groups->members($id) as $member) {
	$member_ids[] = (int)$member["id"];
}

// "Is it granted" and "is the grant read-only" are different questions, and
// the form asks both. Both maps cover every connection, so the template never
// indexes a key nobody set.
$mode = array();
foreach ($groups->grants($id) as $grant) {
	$mode[(int)$grant["id"]] = (int)$grant["is_readonly"] == 1;
}

$listing = $connections->list_all();
$granted = array();
$readonly = array();
foreach ($listing as $connection) {
	$connection_id = (int)$connection["id"];
	$granted[$connection_id] = array_key_exists($connection_id, $mode);
	$readonly[$connection_id] = $granted[$connection_id] && $mode[$connection_id];
}

render($tpl, "Group: " . $group["name"], "pane_group.tpl", $ctx, $panel, array(
	"group" => $group,
	"policies" => user_dao::policies(),
	"users" => $users->list_all(),
	"member_ids" => $member_ids,
	"connections" => $listing,
	"granted" => $granted,
	"readonly" => $readonly,
	"errors" => $errors,
));
