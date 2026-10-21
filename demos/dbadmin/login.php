<?php

// @route: /login

// GET renders the form and POST answers it, from one file: `@route:` with no
// method routes both, and the two halves of a sign-in are one page.
//
// setup.php and not bootstrap.php, because bootstrap.php is the gate, and the
// gate redirects a visitor without a session to this page.
include "setup.php";

// A visitor who is already signed in has nothing to do here.
if ($session->valid()) {
	redirect_to("/");
}

$title = "Sign in";
$message = "";

// A tree with no .htpasswd has no accounts at all. The page says that, where a
// form would refuse every password and name no cause. A checkout whose copy of
// the file went missing lands in the same state.
$configured = $auth->exists();
if (!$configured) {
	$message = "No user database: create .htpasswd in the application root with htpasswd -B -C 10 .htpasswd admin";
}

if ($configured && $_SERVER["REQUEST_METHOD"] == "POST") {
	$username = "";
	if (isset($_POST["username"])) {
		$username = trim($_POST["username"]);
	}

	$password = "";
	if (isset($_POST["password"])) {
		$password = $_POST["password"];
	}

	// One call does the whole of it: the manager puts the credential to the
	// password file and starts a session for the user it names. What comes
	// back is a boolean, so this page leaks nothing about which half of the
	// credential missed.
	if ($session->login($username, $password)) {
		redirect_to("/");
	}

	$message = "Those credentials were not accepted.";
}

$tpl->load("login.tpl");
$tpl->assign(array("title" => $title, "message" => $message, "configured" => $configured));
$tpl->render();
$db->close();
