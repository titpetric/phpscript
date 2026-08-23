<?php

// @route GET /admin/user
// @route POST /admin/user
// @route GET /admin/user/{id}
// @route POST /admin/user/{id}
// @route POST /admin/user/{id}/delete

include "bootstrap.php";

require_admin($ctx, $tpl);

$where = path();
$panel = sidebar($ctx, $acl, $connections, $tables, "");
$errors = array();

if ($where === "/admin/user") {
	if (is_post()) {
		require_csrf($ctx, $tpl);

		try {
			$id = $users->create($ctx, post("username", ""), post("password", ""), checked("is_admin"), post("policy", "toggle"));

			$sessions->set_flash($ctx, "User created.");
			redirect_to("/admin/user/" . (string)$id);
		} catch (Exception $e) {
			$errors[] = $e->getMessage();
		}
	}

	render($tpl, "Users", "pane_users.tpl", $ctx, $panel, array(
		"users" => $users->list_all(),
		"policies" => user_dao::policies(),
		"errors" => $errors,
		"username" => post("username", ""),
	));
	exit();
}

$id = (int)$_PATH["id"];
$user = $users->find($id);
if (!$user) {
	fail($ctx, $tpl, 404, "No such user.");
}

if (str_ends_with($where, "/delete")) {
	require_csrf($ctx, $tpl);

	if ($id == $ctx["user_id"]) {
		fail($ctx, $tpl, 409, "You cannot delete the account you are signed in as.");
	}

	try {
		$users->remove($ctx, $id);
	} catch (Exception $e) {
		fail($ctx, $tpl, 409, $e->getMessage());
	}

	$sessions->set_flash($ctx, "User " . $user["username"] . " deleted.");
	redirect_to("/admin/user");
}

if (is_post()) {
	require_csrf($ctx, $tpl);

	$action = post("action", "profile");

	try {
		if ($action === "password") {
			$users->set_password($ctx, $id, post("password", ""));
			$sessions->set_flash($ctx, "Password changed; that account's sessions were signed out.");
		} elseif ($action === "groups") {
			$chosen = array();
			foreach ($groups->list_all() as $group) {
				if (checked("group_" . (string)$group["id"])) {
					$chosen[] = (int)$group["id"];
				}
			}

			$users->set_groups($ctx, $id, $chosen);
			$sessions->set_flash($ctx, "Group membership updated.");
		} else {
			$users->update_profile($ctx, $id, checked("is_admin"), checked("is_enabled"), post("policy", "toggle"));
			$sessions->set_flash($ctx, "User updated.");
		}

		redirect_to("/admin/user/" . (string)$id);
	} catch (Exception $e) {
		$errors[] = $e->getMessage();
	}

	$user = $users->find($id);
}

$member_of = array();
foreach ($users->groups($id) as $group) {
	$member_of[] = (int)$group["id"];
}

render($tpl, "User: " . $user["username"], "pane_user.tpl", $ctx, $panel, array(
	"user" => $user,
	"policies" => user_dao::policies(),
	"groups" => $groups->list_all(),
	"member_of" => $member_of,
	"sessions" => $sessions->for_user($id),
	"log" => $audit->history("user", $id, 20),
	"errors" => $errors,
	"is_self" => $id == $ctx["user_id"],
));
