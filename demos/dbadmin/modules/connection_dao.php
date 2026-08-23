<?php

/**
 * connection_dao stores the named target databases and opens them.
 *
 * A connection normally comes from the environment the host was started with.
 * These come from a table, so open() registers the DSN with the runtime before
 * asking for it: Database::register() writes into the provider new Database()
 * resolves through, and a virtual host has its own, so one site's connections
 * are not another's.
 *
 * dsn holds credentials in cleartext. harden.php restricts the metadata file,
 * redact_dsn() strips the userinfo before anything reaches a template, and
 * tables_dao hides dbadmin's own tables when a connection points back here.
 */
class connection_dao {
	public $db;
	public $audit;

	function __construct($audit) {
		$this->db = new Database("dbadmin");
		$this->audit = $audit;
	}

	/** COLUMNS is the select list every read of a connection shares. */
	static function columns() {
		return "id, name, driver, dsn, default_schema, is_enabled, is_readonly, status, status_message," . " table_count, column_count, schema_count," . " strftime('%Y-%m-%d %H:%M:%S', checked_at) AS checked_at," . " strftime('%Y-%m-%d %H:%M:%S', created_at) AS created_at";
	}

	/** find returns the connection with id $id, or false. */
	function find($id) {
		return $this->db->get("SELECT " . connection_dao::columns() . " FROM connection WHERE id = ?", (int)$id);
	}

	/** find_by_name returns the connection named $name, or false. */
	function find_by_name($name) {
		return $this->db->get("SELECT " . connection_dao::columns() . " FROM connection WHERE name = ?", strtolower((string)$name));
	}

	/** list_all returns every connection, enabled or not. */
	function list_all() {
		return $this->db->get_all("SELECT " . connection_dao::columns() . "," . " (SELECT COUNT(*) FROM user_group_connection gc WHERE gc.connection_id = connection.id) AS grants" . " FROM connection ORDER BY name");
	}

	/** grants returns the groups that reach connection $id. */
	function grants($id) {
		return $this->db->get_all("SELECT g.id, g.name, gc.is_readonly FROM user_group_connection gc" . " JOIN user_group g ON g.id = gc.user_group_id WHERE gc.connection_id = ? ORDER BY g.name", (int)$id);
	}

	/** create adds a connection and returns its id. */
	function create($ctx, $name, $dsn, $schema, $is_readonly) {
		$name = strtolower(trim((string)$name));
		$driver = driver_dao::driver_of($dsn);

		$errors = connection_dao::validate($name, $dsn);
		if (count($errors) > 0) {
			throw new Exception(implode(" ", $errors), 400);
		}

		try {
			$this->db->insert("connection", array(
				"name" => $name,
				"driver" => $driver,
				"dsn" => (string)$dsn,
				"default_schema" => connection_dao::default_schema($driver, $schema),
				"is_enabled" => 1,
				"is_readonly" => $is_readonly ? 1 : 0,
			));
		} catch (Exception $e) {
			if (str_contains($e->getMessage(), "UNIQUE constraint failed")) {
				throw new Exception("A connection named " . $name . " already exists.", 409);
			}

			throw $e;
		}

		$id = (int)$this->db->insert_id();

		$this->audit->log($ctx, "insert", "connection", $id, "created connection: " . $name, array("driver" => $driver, "dsn" => connection_dao::redact_dsn($dsn)));

		return $id;
	}

	/** update changes a connection's DSN, schema and mode. */
	function update($ctx, $id, $dsn, $schema, $is_enabled, $is_readonly) {
		$connection = $this->find($id);
		if (!$connection) {
			throw new Exception("No such connection.", 404);
		}

		$driver = driver_dao::driver_of($dsn);
		$errors = connection_dao::validate($connection["name"], $dsn);
		if (count($errors) > 0) {
			throw new Exception(implode(" ", $errors), 400);
		}

		$this->db->query("UPDATE connection SET driver = ?, dsn = ?, default_schema = ?, is_enabled = ?, is_readonly = ?," . " status = 'unknown', status_message = '', updated_at = CURRENT_TIMESTAMP WHERE id = ?", $driver, (string)$dsn, connection_dao::default_schema($driver, $schema), $is_enabled ? 1 : 0, $is_readonly ? 1 : 0, (int)$id);

		$this->audit->log($ctx, "update", "connection", $id, "updated connection: " . $connection["name"], array(
			"driver" => $driver,
			"dsn" => connection_dao::redact_dsn($dsn),
			"is_enabled" => $is_enabled ? 1 : 0,
		));
	}

	/** remove deletes a connection and every grant and session pointing at it. */
	function remove($ctx, $id, $groups, $sessions) {
		$connection = $this->find($id);
		if (!$connection) {
			throw new Exception("No such connection.", 404);
		}

		$groups->detach($id);
		$sessions->detach($id);
		$this->db->query("DELETE FROM connection WHERE id = ?", (int)$id);

		$this->audit->log($ctx, "delete", "connection", $id, "deleted connection: " . $connection["name"], array());
	}

	/**
	 * open returns a client for $connection, read-only when $readonly.
	 *
	 * The DSN is registered on every call. Registration is idempotent for an
	 * unchanged DSN and closes the old pool for a changed one, so an edited
	 * connection takes effect on the next request rather than at the next
	 * restart.
	 *
	 * A failure is returned rather than thrown. Half the callers are the test
	 * page, which exists to display exactly this.
	 */
	function open($connection, $readonly) {
		$driver = (string)$connection["driver"];
		if (!driver_dao::supported($driver)) {
			return array("error" => "Unsupported driver: " . $driver);
		}

		try {
			Database::register($connection["name"], $connection["dsn"]);
			$db = new Database($connection["name"]);
		} catch (Exception $e) {
			return array("error" => $e->getMessage());
		}

		if ($readonly || (int)$connection["is_readonly"] == 1) {
			$db->is_readonly = true;
		}

		return array("db" => $db, "driver" => $driver);
	}

	/**
	 * test probes $connection and records what it found.
	 *
	 * The liveness probe runs first, so a connection that cannot be reached
	 * reports the driver's own message rather than a counting query failing
	 * for a reason nobody can act on.
	 */
	function test($connection, $tables) {
		$opened = $this->open($connection, true);
		if (array_key_exists("error", $opened)) {
			$this->record($connection["id"], "error", $opened["error"], 0, 0, 0);
			return array(
				"status" => "error",
				"message" => connection_dao::first_line($opened["error"]),
				"tables" => 0,
				"columns" => 0,
				"schemas" => 0,
			);
		}

		$db = $opened["db"];
		$driver = $opened["driver"];
		$schema = (string)$connection["default_schema"];

		try {
			$tables->probe($db, $driver);
			$counts = $tables->counts($db, $driver, $schema);
		} catch (Exception $e) {
			$this->record($connection["id"], "error", $e->getMessage(), 0, 0, 0);
			return array(
				"status" => "error",
				"message" => connection_dao::first_line($e->getMessage()),
				"tables" => 0,
				"columns" => 0,
				"schemas" => 0,
			);
		}

		$this->record($connection["id"], "ok", "", $counts["tables"], $counts["columns"], $counts["schemas"]);

		return array(
			"status" => "ok",
			"message" => "",
			"tables" => $counts["tables"],
			"columns" => $counts["columns"],
			"schemas" => $counts["schemas"],
		);
	}

	/** record caches what the last test saw. */
	function record($id, $status, $message, $tables, $columns, $schemas) {
		$this->db->query("UPDATE connection SET status = ?, status_message = ?, table_count = ?, column_count = ?," . " schema_count = ?, checked_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?", $status, connection_dao::first_line($message), (int)$tables, (int)$columns, (int)$schemas, (int)$id);
	}

	/**
	 * first_line returns the first line of $message, capped.
	 *
	 * A driver error arrives wrapped: the client appends the statement it
	 * failed on, and a connection failure repeats the reason once per name it
	 * was tried under. The first line is the reason; the rest is provenance
	 * that belongs in the log rather than in a table cell.
	 */
	static function first_line($message) {
		$text = trim((string)$message);
		$break = strpos($text, "\n");
		if ($break !== false) {
			$text = trim(substr($text, 0, $break));
		}

		return substr($text, 0, 500);
	}

	/**
	 * redact_dsn removes the credentials from $dsn.
	 *
	 * Everything between "://" and the last "@" of the authority is the
	 * userinfo, and it is the only part of a DSN nobody needs to read.
	 */
	static function redact_dsn($dsn) {
		$dsn = (string)$dsn;
		$scheme = strpos($dsn, "://");
		if ($scheme === false) {
			return connection_dao::redact_pairs($dsn);
		}

		$head = substr($dsn, 0, $scheme + 3);
		$rest = substr($dsn, $scheme + 3);

		$at = strpos($rest, "@");
		if ($at === false) {
			return $head . connection_dao::redact_pairs($rest);
		}

		return $head . "***@" . connection_dao::redact_pairs(substr($rest, $at + 1));
	}

	/** redact_pairs blanks a password= parameter, which mysql DSNs also use. */
	static function redact_pairs($text) {
		return preg_replace("/(password|pwd)=[^&; ]*/", "\\1=***", $text);
	}

	/** default_schema returns the schema to browse when none is chosen. */
	static function default_schema($driver, $schema) {
		$schema = trim((string)$schema);
		if ($driver === "sqlite") {
			return "";
		}

		if ($schema === "" && $driver === "postgres") {
			return "public";
		}

		return $schema;
	}

	/** validate returns the reasons $name and $dsn are unacceptable. */
	static function validate($name, $dsn) {
		$errors = array();

		if (preg_match("/^[a-z0-9_-]{2,32}$/", (string)$name) != 1) {
			$errors[] = "A connection name is 2 to 32 characters of lowercase letters, digits, dash or underscore.";
		}

		if ($name === "dbadmin") {
			$errors[] = "The name dbadmin is reserved for this application's own database.";
		}

		$driver = driver_dao::driver_of($dsn);
		if ($driver === "") {
			$errors[] = "A DSN starts with the driver, as in sqlite://file.db or postgres://user:pass@host/db.";
		} elseif (!driver_dao::supported($driver)) {
			$errors[] = "Unsupported driver: " . $driver . ". Use sqlite, mysql or postgres.";
		}

		return $errors;
	}
}
