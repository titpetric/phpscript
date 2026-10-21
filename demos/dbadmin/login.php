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

	// The token check answers first and the rate limit second, so a post
	// refused by either costs no bcrypt derivation. The limit refuses a
	// correct password too: it bounds what the route spends before anything
	// reads the credential, and a guessing client learns nothing by mixing a
	// candidate in.
	if (!csrf_check()) {
		http_response_code(403);
		$message = "The request did not carry the sign-in form's token. Reload the page and sign in again.";
	} else if (login_limited($db, login_address(), $username)) {
		http_response_code(429);
		$message = "Too many sign-in attempts. Wait ten seconds and try again.";
	} else if ($session->login($username, $password)) {
		// One call does the whole of it: the manager puts the credential to
		// the password file and starts a session for the user it names. An
		// accepted credential clears the refusals it outweighs, so a user who
		// mistyped twice and then signed in starts the next window at zero.
		$db->query("DELETE FROM login_attempt WHERE address = ? OR username = ?", login_address(), $username);
		redirect_to("/");
	} else {
		// What comes back from login() is a boolean, so this page leaks
		// nothing about which half of the credential missed.
		$db->query("INSERT INTO login_attempt (address, username, attempted_at) VALUES (?, ?, ?)", login_address(), $username, time());
		$message = "Those credentials were not accepted.";
	}
}

$tpl->load("login.tpl");
$tpl->assign(array("title" => $title, "message" => $message, "configured" => $configured));
$tpl->render();
$db->close();

/**
 * Whether the posting address, or the account it names, spent its five
 * refusals in the last ten seconds.
 *
 * The ceiling exists because a bcrypt derivation at cost 10 is 53 ms of one
 * core (docs/reference/extensions/hashing.md prices it), so without one an
 * unauthenticated caller sets the rate at which this server burns CPU. Five
 * per ten seconds caps one address at about three percent of a core. Counting
 * per account as well stops a caller rotating addresses against one name.
 */
function login_limited($db, $address, $username) {
	$db->query("DELETE FROM login_attempt WHERE attempted_at < ?", time() - 10);
	$row = $db->get("SELECT COUNT(*) AS refused FROM login_attempt WHERE address = ? OR username = ?", $address, $username);
	return $row["refused"] >= 5;
}

/**
 * The posting client's address: one of the two keys the rate limit counts
 * refusals under.
 */
function login_address() {
	if (isset($_SERVER["REMOTE_ADDR"])) {
		return $_SERVER["REMOTE_ADDR"];
	}

	return "";
}
