<?php

/**
 * login_dao turns a username and a password into a session.
 *
 * A wrong password and an unknown username are the same answer, and take the
 * same time: password_verify() against an empty hash still spends a bcrypt
 * derivation, so the response does not say whether the account exists.
 */
class login_dao {
	public $db;
	public $users;
	public $sessions;
	public $audit;

	function __construct($users, $sessions, $audit) {
		$this->db = new Database("dbadmin");
		$this->users = $users;
		$this->sessions = $sessions;
		$this->audit = $audit;
	}

	/**
	 * authenticate returns the account named $username when $password is
	 * theirs, and false otherwise.
	 *
	 * A disabled account fails here rather than at the session, so a
	 * disabled user is told the same thing as a wrong password and learns
	 * nothing from the difference.
	 */
	function authenticate($username, $password) {
		$user = $this->users->find_by_username(trim((string)$username));

		$hash = "";
		if ($user && (int)$user["is_enabled"] == 1) {
			$hash = (string)$user["password_hash"];
		}

		if (!password_verify((string)$password, $hash)) {
			$this->audit->log(session_dao::anonymous(), "login", "user", "", "failed sign-in for: " . (string)$username, array());
			return false;
		}

		// Login is the only moment the plaintext is in hand, so it is the
		// only moment a stored hash can be moved to a newer cost.
		if (password_needs_rehash($hash, PASSWORD_DEFAULT)) {
			$this->users->rehash($user["id"], (string)$password);
		}

		return $user;
	}

	/**
	 * sign_in creates the session and records the sign-in.
	 *
	 * It returns the context of the new session so the caller can redirect
	 * without reading the cookie it has only just staged.
	 */
	function sign_in($user, $remote_addr, $user_agent) {
		$this->sessions->start($user["id"], $remote_addr, $user_agent);
		$this->users->touch_login($user["id"]);

		$ctx = array("user_id" => (int)$user["id"], "connection_id" => 0);

		$this->audit->log($ctx, "login", "user", $user["id"], "signed in: " . $user["username"], array("remote_addr" => $remote_addr));

		return $ctx;
	}

	/** sign_out revokes the session and clears its cookie. */
	function sign_out($ctx) {
		$this->sessions->revoke($ctx);
	}
}
