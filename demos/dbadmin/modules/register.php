<?php

// @route GET /register
// @route POST /register

include "bootstrap.php";

$open = $registration->is_open();

// Once an account exists, creating another is an administrative act and lives
// under /admin/user. Leaving this route reachable but refusing is better than
// a 404: it tells the operator the installation is already claimed.
if (!$open) {
	if ($ctx["logged_in"]) {
		redirect_to("/admin/user");
	}

	fail($ctx, $tpl, 403, "This installation already has an administrator. Sign in instead.");
}

$errors = array();
$username = post("username", "");

if (is_post()) {
	$password = post("password", "");
	$confirm = post("confirm", "");

	if ($password !== $confirm) {
		$errors[] = "The two passwords do not match.";
	} else {
		try {
			$id = $registration->claim($username, $password);
			$user = $users->find($id);

			$login->sign_in($user, remote_addr(), user_agent());
			redirect_to("/");
		} catch (Exception $e) {
			$errors[] = $e->getMessage();
		}
	}
}

render($tpl, "Set up dbadmin", "pane_register.tpl", $ctx, sidebar_off(), array(
	"errors" => $errors,
	"username" => $username,
	"standalone" => true,
));
