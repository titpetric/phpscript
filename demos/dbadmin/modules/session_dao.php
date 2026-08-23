<?php

/**
 * session_dao carries the state of one logged-in browser.
 *
 * Session\Manager stores exactly one opaque string and its only writer is
 * start(), which mints a fresh cookie every time it is called. So the string
 * it stores is a token, an immutable pointer at a user_session row, and every
 * mutable bit of session state is a column on that row. Switching connection
 * or turning destructive mode on is an UPDATE and the cookie is untouched.
 *
 * Nothing here formats or parses a timestamp: there is no date() in this
 * runtime, so expiry is decided by the comparison SQLite makes.
 */
class session_dao {
	/** LIFETIME is how long a session lasts, as a SQLite interval. */
	const LIFETIME = "+12 hours";

	/** DESTRUCTIVE_WINDOW is how long destructive mode stays on for. */
	const DESTRUCTIVE_WINDOW = "+900 seconds";

	public $db;
	public $audit;
	public $manager;

	function __construct($audit) {
		$this->db = new Database("dbadmin");
		$this->audit = $audit;
		$this->manager = new Session\Manager(new Session\Storage\Disk);
	}

	/**
	 * current returns the context of the request's session, or the anonymous
	 * context when there is none.
	 *
	 * A cookie that no longer resolves to a live row is not an error: a
	 * revoked, expired or deleted session is simply not logged in, and the
	 * page it lands on is the login form.
	 */
	function current() {
		$token = $this->token();
		if ($token === "") {
			return session_dao::anonymous();
		}

		$row = $this->db->get("SELECT s.id, s.token, s.csrf_token, s.user_id, s.connection_id, s.schema_name, s.flash," . " (s.is_destructive = 1 AND s.destructive_until > CURRENT_TIMESTAMP) AS is_destructive," . " strftime('%Y-%m-%dT%H:%M:%SZ', s.destructive_until) AS destructive_until," . " u.username, u.is_admin, u.destructive_policy," . " c.name AS connection_name, c.driver AS driver, c.default_schema AS default_schema," . " c.is_readonly AS connection_readonly, c.is_enabled AS connection_enabled" . " FROM user_session s" . " JOIN user u ON u.id = s.user_id" . " LEFT JOIN connection c ON c.id = s.connection_id" . " WHERE s.token = ? AND s.is_revoked = 0 AND s.expires_at > CURRENT_TIMESTAMP AND u.is_enabled = 1", $token);

		if (!$row) {
			return session_dao::anonymous();
		}

		$schema = (string)$row["schema_name"];
		if ($schema === "" && $row["default_schema"] !== null) {
			$schema = (string)$row["default_schema"];
		}

		// A connection disabled while a session was on it is no connection.
		$connection_id = (int)$row["connection_id"];
		$connection_name = "";
		$driver = "";
		if ($connection_id > 0 && (int)$row["connection_enabled"] == 1) {
			$connection_name = (string)$row["connection_name"];
			$driver = (string)$row["driver"];
		} else {
			$connection_id = 0;
			$schema = "";
		}

		return array(
			"logged_in" => true,
			"session_id" => (int)$row["id"],
			"token" => $token,
			"csrf_token" => (string)$row["csrf_token"],
			"user_id" => (int)$row["user_id"],
			"username" => (string)$row["username"],
			"is_admin" => (int)$row["is_admin"] == 1,
			"destructive_policy" => (string)$row["destructive_policy"],
			"connection_id" => $connection_id,
			"connection_name" => $connection_name,
			"driver" => $driver,
			"schema_name" => $schema,
			"connection_readonly" => (int)$row["connection_readonly"] == 1,
			"is_destructive" => (int)$row["is_destructive"] == 1,
			"destructive_until" => (string)$row["destructive_until"],
			"flash" => (string)$row["flash"],
		);
	}

	/** anonymous is the context of a request with no session. */
	static function anonymous() {
		return array(
			"logged_in" => false,
			"session_id" => 0,
			"token" => "",
			"csrf_token" => "",
			"user_id" => 0,
			"username" => "",
			"is_admin" => false,
			"destructive_policy" => "denied",
			"connection_id" => 0,
			"connection_name" => "",
			"driver" => "",
			"schema_name" => "",
			"connection_readonly" => true,
			"is_destructive" => false,
			"destructive_until" => "",
			"flash" => "",
		);
	}

	/**
	 * token returns the token the request's cookie points at, or "".
	 *
	 * valid() is asked first because get() treats a missing cookie as an
	 * error, and a first visit is not an error.
	 */
	function token() {
		try {
			if (!$this->manager->valid()) {
				return "";
			}

			return (string)$this->manager->get();
		} catch (Exception $e) {
			return "";
		}
	}

	/**
	 * start creates a session for $user_id and stages its cookie.
	 *
	 * The token and the CSRF token are minted by SQLite, because this
	 * runtime has no source of randomness. The cookie itself is 32 bytes
	 * from the host's CSPRNG and is what actually guards the session; the
	 * token only has to be unguessable enough that a session file surviving
	 * a rebuilt database cannot land on a recycled row.
	 */
	function start($user_id, $remote_addr, $user_agent) {
		$this->db->query("INSERT INTO user_session (token, csrf_token, user_id, remote_addr, user_agent, expires_at, created_at, updated_at)" . " VALUES (lower(hex(randomblob(16))), lower(hex(randomblob(16))), ?, ?, ?, datetime('now', '" . session_dao::LIFETIME . "'), CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", (int)$user_id, (string)$remote_addr, substr((string)$user_agent, 0, 255));

		$row = $this->db->get("SELECT token FROM user_session WHERE id = ?", (int)$this->db->insert_id());
		$token = (string)$row["token"];

		$this->manager->start($token);
		return $token;
	}

	/**
	 * revoke ends the session and rotates the cookie onto a value no row
	 * matches.
	 *
	 * The binding has no way to destroy a session, so the cookie is replaced
	 * rather than removed. The row is kept and marked instead of deleted, so
	 * a replayed cookie is answered by a row that says no.
	 */
	function revoke($ctx) {
		if ($ctx["session_id"] > 0) {
			$this->db->query("UPDATE user_session SET is_revoked = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?", (int)$ctx["session_id"]);
			$this->audit->log($ctx, "logout", "user_session", $ctx["session_id"], "signed out: " . $ctx["username"], array());
		}

		$this->manager->start("");
	}

	/** revoke_all ends every session belonging to $user_id. */
	function revoke_all($user_id) {
		$this->db->query("UPDATE user_session SET is_revoked = 1, updated_at = CURRENT_TIMESTAMP WHERE user_id = ?", (int)$user_id);
	}

	/**
	 * set_connection points the session at $connection_id and $schema.
	 *
	 * Destructive mode is cleared by the move. It was granted for the
	 * database the user was looking at, and carrying it into the next one is
	 * exactly the mistake the toggle exists to prevent.
	 */
	function set_connection($ctx, $connection_id, $schema) {
		$this->db->query("UPDATE user_session SET connection_id = ?, schema_name = ?, is_destructive = 0," . " destructive_until = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?", (int)$connection_id, (string)$schema, (int)$ctx["session_id"]);
	}

	/** set_schema changes the browsed schema without changing connection. */
	function set_schema($ctx, $schema) {
		$this->db->query("UPDATE user_session SET schema_name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", (string)$schema, (int)$ctx["session_id"]);
	}

	/**
	 * set_destructive turns destructive mode on for a fixed window, or off.
	 *
	 * The window is written and compared in SQL. Turning it on again while
	 * it is on restarts it, which is what a user reaching for the toggle a
	 * second time means by it.
	 */
	function set_destructive($ctx, $on) {
		if ($on) {
			$this->db->query("UPDATE user_session SET is_destructive = 1," . " destructive_until = datetime('now', '" . session_dao::DESTRUCTIVE_WINDOW . "')," . " updated_at = CURRENT_TIMESTAMP WHERE id = ?", (int)$ctx["session_id"]);
		} else {
			$this->db->query("UPDATE user_session SET is_destructive = 0, destructive_until = NULL," . " updated_at = CURRENT_TIMESTAMP WHERE id = ?", (int)$ctx["session_id"]);
		}

		$this->audit->log($ctx, "admin", "user_session", $ctx["session_id"], $on ? "enabled destructive mode" : "left destructive mode", array());
	}

	/**
	 * set_flash leaves a message for the page the browser is redirected to.
	 *
	 * There is no setcookie() in this runtime and the session cookie is
	 * spoken for, so the message travels in the session row rather than in
	 * the URL. That also keeps it off the screen if the user shares the link.
	 */
	function set_flash($ctx, $message) {
		if ($ctx["session_id"] == 0) {
			return;
		}

		$this->db->query("UPDATE user_session SET flash = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", (string)$message, (int)$ctx["session_id"]);
	}

	/** take_flash returns the pending message and clears it. */
	function take_flash($ctx) {
		$message = (string)$ctx["flash"];
		if ($message !== "" && $ctx["session_id"] > 0) {
			$this->db->query("UPDATE user_session SET flash = '' WHERE id = ?", (int)$ctx["session_id"]);
		}

		return $message;
	}

	/** for_user returns the live sessions of $user_id, newest first. */
	function for_user($user_id) {
		return $this->db->get_all("SELECT id, remote_addr, user_agent," . " strftime('%Y-%m-%d %H:%M:%S', created_at) AS created_at," . " strftime('%Y-%m-%d %H:%M:%S', expires_at) AS expires_at" . " FROM user_session WHERE user_id = ? AND is_revoked = 0 AND expires_at > CURRENT_TIMESTAMP" . " ORDER BY created_at DESC", (int)$user_id);
	}

	/** detach clears the connection of every session pointing at $connection_id. */
	function detach($connection_id) {
		$this->db->query("UPDATE user_session SET connection_id = 0, schema_name = '', is_destructive = 0," . " destructive_until = NULL, updated_at = CURRENT_TIMESTAMP WHERE connection_id = ?", (int)$connection_id);
	}

	/** prune deletes sessions that expired more than a day ago. */
	function prune() {
		$this->db->query("DELETE FROM user_session WHERE expires_at < datetime('now', '-1 day')");
	}
}
