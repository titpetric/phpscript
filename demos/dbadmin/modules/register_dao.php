<?php

/**
 * register_dao decides who is allowed to create an account.
 *
 * dbadmin starts with no accounts and no way to sign in, so the first
 * registration has to be open. It is also the one that matters most, which is
 * why it is the one that becomes the administrator: an installation that has
 * been reached once is an installation with an owner.
 *
 * After that the route is administrative. is_open() is asked on every GET
 * rather than cached, because the answer changes exactly once and the query it
 * costs is a covered index lookup.
 */
class register_dao {
	public $db;
	public $users;
	public $audit;

	function __construct($users, $audit) {
		$this->db = new Database("dbadmin");
		$this->users = $users;
		$this->audit = $audit;
	}

	/** is_open reports whether registration is available without a session. */
	function is_open() {
		return $this->users->count_all() == 0;
	}

	/**
	 * claim creates the first account, as an administrator.
	 *
	 * The count is checked again inside the transaction that inserts, so two
	 * browsers racing for an empty installation cannot both become the
	 * administrator: the unique index settles the username, and the count
	 * settles the role.
	 */
	function claim($username, $password) {
		if (!$this->is_open()) {
			throw new Exception("An administrator already exists. Sign in instead.", 403);
		}

		$id = $this->users->create(session_dao::anonymous(), $username, $password, true, "toggle");

		$this->audit->log(array("user_id" => $id, "connection_id" => 0), "admin", "user", $id, "claimed this installation as its administrator: " . $username, array());

		return $id;
	}
}
