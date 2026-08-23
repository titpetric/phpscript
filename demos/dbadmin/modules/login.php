<?php

// @route GET /login
// @route POST /login
// @route POST /logout

include "bootstrap.php";

if (path() === "/logout") {
	require_csrf($ctx, $tpl);
	$login->sign_out($ctx);
	redirect_to("/login");
}

// An installation with no accounts has nothing to sign in to; send the first
// visitor to the page that makes them the administrator.
if ($registration->is_open()) {
	redirect_to("/register");
}

if ($ctx["logged_in"] && !is_post()) {
	redirect_to("/");
}

$errors = array();
$username = post("username", "");

if (is_post()) {
	$user = $login->authenticate($username, post("password", ""));
	if ($user) {
		$login->sign_in($user, remote_addr(), user_agent());
		redirect_to("/");
	}

	// One message for a wrong password, an unknown name and a disabled
	// account. Which of the three it was is not the browser's business.
	$errors[] = "Those credentials were not accepted.";
}

render($tpl, "Sign in", "pane_login.tpl", $ctx, sidebar_off(), array(
	"errors" => $errors,
	"username" => $username,
	"standalone" => true,
));
