<?php

/**
 * user_dao stores accounts.
 *
 * The first account created is the administrator; after that, creating one is
 * an administrative act and is audited as such.
 */
class user_dao {
	public $db;
	public $audit;

	function __construct($audit) {
		$this->db = new Database("dbadmin");
		$this->audit = $audit;
	}

	/** POLICIES are the values user.destructive_policy accepts, loosest last. */
	static function policies() {
		return array("denied", "toggle", "allowed");
	}

	/** find returns the account with id $id, or false. */
	function find($id) {
		return $this->db->get("SELECT id, username, password_hash, is_admin, is_enabled, destructive_policy," . " strftime('%Y-%m-%d %H:%M:%S', last_login_at) AS last_login_at," . " strftime('%Y-%m-%d %H:%M:%S', created_at) AS created_at" . " FROM user WHERE id = ?", (int)$id);
	}

	/** find_by_username returns the account named $username, or false. */
	function find_by_username($username) {
		return $this->db->get("SELECT id, username, password_hash, is_admin, is_enabled, destructive_policy" . " FROM user WHERE username = ?", (string)$username);
	}

	/** count_all returns how many accounts exist. */
	function count_all() {
		$row = $this->db->get("SELECT COUNT(*) AS total FROM user");
		return (int)$row["total"];
	}

	/**
	 * list_all returns every account with the groups it belongs to, as a
	 * comma separated string.
	 *
	 * One statement rather than one per user: the admin list is the only
	 * place both are wanted, and group_concat keeps it a single round trip.
	 */
	function list_all() {
		return $this->db->get_all("SELECT u.id, u.username, u.is_admin, u.is_enabled, u.destructive_policy," . " strftime('%Y-%m-%d %H:%M:%S', u.last_login_at) AS last_login_at," . " (SELECT COUNT(*) FROM user_session s WHERE s.user_id = u.id AND s.is_revoked = 0 AND s.expires_at > CURRENT_TIMESTAMP) AS sessions," . " (SELECT group_concat(g.name, ', ') FROM user_group_member m" . "  JOIN user_group g ON g.id = m.user_group_id WHERE m.user_id = u.id) AS groups" . " FROM user u ORDER BY u.username");
	}

	/**
	 * create adds an account and returns its id.
	 *
	 * The unique index on username is what enforces uniqueness. Asking first
	 * and inserting second is a race, and the answer to it is already in the
	 * schema, so the duplicate is caught rather than predicted.
	 */
	function create($ctx, $username, $password, $is_admin, $policy) {
		$errors = user_dao::validate($username, $password, $policy);
		if (count($errors) > 0) {
			throw new Exception(implode(" ", $errors), 400);
		}

		try {
			$this->db->insert("user", array(
				"username" => $username,
				"password_hash" => password_hash($password, PASSWORD_DEFAULT),
				"is_admin" => $is_admin ? 1 : 0,
				"is_enabled" => 1,
				"destructive_policy" => $policy,
			));
		} catch (Exception $e) {
			if (str_contains($e->getMessage(), "UNIQUE constraint failed")) {
				throw new Exception("The username " . $username . " is already taken.", 409);
			}

			throw $e;
		}

		$id = (int)$this->db->insert_id();

		$what = "created new user: " . $username;
		if ($is_admin) {
			$what = "created new administrator: " . $username;
		}

		$this->audit->log($ctx, "insert", "user", $id, $what, array("username" => $username, "is_admin" => $is_admin ? 1 : 0));

		return $id;
	}

	/** update_profile changes everything about an account except its password. */
	function update_profile($ctx, $id, $is_admin, $is_enabled, $policy) {
		if (!in_array($policy, user_dao::policies())) {
			throw new Exception("Unknown destructive policy: " . (string)$policy, 400);
		}

		$user = $this->find($id);
		if (!$user) {
			throw new Exception("No such user.", 404);
		}

		// The last administrator cannot demote or disable themselves out of
		// existence; there would be nobody left who could put one back.
		if ((!$is_admin || !$is_enabled) && $this->count_admins() < 2 && (int)$user["is_admin"] == 1) {
			throw new Exception("This is the only administrator; promote another account first.", 409);
		}

		$this->db->query("UPDATE user SET is_admin = ?, is_enabled = ?, destructive_policy = ?, updated_at = CURRENT_TIMESTAMP" . " WHERE id = ?", $is_admin ? 1 : 0, $is_enabled ? 1 : 0, $policy, (int)$id);

		if (!$is_enabled) {
			$this->db->query("UPDATE user_session SET is_revoked = 1, updated_at = CURRENT_TIMESTAMP WHERE user_id = ?", (int)$id);
		}

		$this->audit->log($ctx, "update", "user", $id, "updated user: " . $user["username"], array(
			"is_admin" => $is_admin ? 1 : 0,
			"is_enabled" => $is_enabled ? 1 : 0,
			"destructive_policy" => $policy,
		));
	}

	/**
	 * set_password replaces an account's password and revokes its sessions.
	 *
	 * A password change that leaves the old sessions running has not changed
	 * anything for whoever was using them.
	 */
	function set_password($ctx, $id, $password) {
		$user = $this->find($id);
		if (!$user) {
			throw new Exception("No such user.", 404);
		}

		$errors = user_dao::validate_password($password);
		if (count($errors) > 0) {
			throw new Exception(implode(" ", $errors), 400);
		}

		$this->db->query("UPDATE user SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", password_hash($password, PASSWORD_DEFAULT), (int)$id);
		$this->db->query("UPDATE user_session SET is_revoked = 1, updated_at = CURRENT_TIMESTAMP WHERE user_id = ?", (int)$id);

		$this->audit->log($ctx, "update", "user", $id, "changed password for: " . $user["username"], array());
	}

	/** touch_login records a successful sign-in. */
	function touch_login($id) {
		$this->db->query("UPDATE user SET last_login_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?", (int)$id);
	}

	/**
	 * rehash replaces a stored hash whose cost is out of date.
	 *
	 * Login is the only moment the plaintext is available, so it is the only
	 * moment the cost can be raised without asking for it again.
	 */
	function rehash($id, $password) {
		$this->db->query("UPDATE user SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", password_hash($password, PASSWORD_DEFAULT), (int)$id);
	}

	/**
	 * remove deletes an account, its group memberships and its sessions.
	 *
	 * Its audit rows stay. They are the only record the account existed, and
	 * the log renders a deleted user by id.
	 */
	function remove($ctx, $id) {
		$user = $this->find($id);
		if (!$user) {
			throw new Exception("No such user.", 404);
		}

		if ((int)$user["is_admin"] == 1 && $this->count_admins() < 2) {
			throw new Exception("This is the only administrator and cannot be deleted.", 409);
		}

		$this->db->query("DELETE FROM user_group_member WHERE user_id = ?", (int)$id);
		$this->db->query("DELETE FROM user_session WHERE user_id = ?", (int)$id);
		$this->db->query("DELETE FROM user WHERE id = ?", (int)$id);

		$this->audit->log($ctx, "delete", "user", $id, "deleted user: " . $user["username"], array());
	}

	/** count_admins returns how many enabled administrators exist. */
	function count_admins() {
		$row = $this->db->get("SELECT COUNT(*) AS total FROM user WHERE is_admin = 1 AND is_enabled = 1");
		return (int)$row["total"];
	}

	/** groups returns the group ids $id belongs to. */
	function groups($id) {
		$rows = $this->db->get_all("SELECT g.id, g.name, g.destructive_policy FROM user_group_member m" . " JOIN user_group g ON g.id = m.user_group_id WHERE m.user_id = ? ORDER BY g.name", (int)$id);
		return $rows;
	}

	/** set_groups replaces $id's group membership with $group_ids. */
	function set_groups($ctx, $id, $group_ids) {
		$user = $this->find($id);
		if (!$user) {
			throw new Exception("No such user.", 404);
		}

		$this->db->begin();
		$this->db->query("DELETE FROM user_group_member WHERE user_id = ?", (int)$id);
		foreach ($group_ids as $group_id) {
			$this->db->insert("user_group_member", array("user_id" => (int)$id, "user_group_id" => (int)$group_id));
		}

		$this->db->commit();

		$this->audit->log($ctx, "update", "user", $id, "changed groups for: " . $user["username"], array("group_ids" => $group_ids));
	}

	/** validate returns the reasons $username and $password are unacceptable. */
	static function validate($username, $password, $policy) {
		$errors = array();

		if (preg_match("/^[A-Za-z0-9_.-]{2,64}$/", (string)$username) != 1) {
			$errors[] = "A username is 2 to 64 characters of letters, digits, dot, dash or underscore.";
		}

		if (!in_array($policy, user_dao::policies())) {
			$errors[] = "Unknown destructive policy.";
		}

		return array_merge($errors, user_dao::validate_password($password));
	}

	/**
	 * validate_password returns the reasons $password is unacceptable.
	 *
	 * The upper bound is bcrypt's: it hashes the first 72 bytes and ignores
	 * the rest, so a longer password would be silently truncated rather than
	 * stronger.
	 */
	static function validate_password($password) {
		$errors = array();
		$length = strlen((string)$password);

		if ($length < 8) {
			$errors[] = "A password is at least 8 characters.";
		}

		if ($length > 72) {
			$errors[] = "A password is at most 72 bytes; bcrypt ignores anything past that.";
		}

		return $errors;
	}
}
