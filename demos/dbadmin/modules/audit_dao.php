<?php

/**
 * audit_dao writes and reads the audit trail.
 *
 * Every other DAO holds one of these and calls log() after a change it made.
 * It is the leaf of the object graph and composes nothing itself, which is
 * what keeps the graph a tree.
 */
class audit_dao {
	public $db;

	function __construct() {
		$this->db = new Database("dbadmin");
	}

	/**
	 * log records one event.
	 *
	 * $ctx is the request context, for its user_id and connection_id;
	 * passing the whole thing keeps the two of them from being transposed at
	 * a call site. $action is the SQL verb or the kind of event, $rel_table
	 * and $rel_id name what was touched, $message is the sentence a human
	 * reads and $payload is anything worth keeping that is not one of those.
	 */
	function log($ctx, $action, $rel_table, $rel_id, $message, $payload) {
		$encoded = "";
		if ($payload !== null && count($payload) > 0) {
			$encoded = json_encode($payload);
		}

		$this->db->insert("audit", array(
			"user_id" => audit_dao::context_id($ctx, "user_id"),
			"connection_id" => audit_dao::context_id($ctx, "connection_id"),
			"rel_table" => (string)$rel_table,
			"rel_id" => (string)$rel_id,
			"action" => $action,
			"message" => (string)$message,
			"payload" => $encoded,
		));
	}

	/**
	 * log_statement records a write to a target database, taking its action
	 * from the statement itself.
	 *
	 * An unrecognised leading keyword is recorded as 'admin' rather than
	 * dropped: the audit table has a CHECK constraint on action, and losing
	 * the row would be a worse answer than losing the verb.
	 */
	function log_statement($ctx, $sql, $rel_table, $rel_id, $message, $payload) {
		$action = driver_dao::verb($sql);
		if (!in_array($action, audit_dao::actions())) {
			$action = "admin";
		}

		$this->log($ctx, $action, $rel_table, $rel_id, $message, $payload);
	}

	/** actions returns the values audit.action accepts. */
	static function actions() {
		return array(
			"select",
			"insert",
			"update",
			"delete",
			"truncate",
			"drop",
			"create",
			"alter",
			"login",
			"logout",
			"denied",
			"admin",
		);
	}

	/** context_id reads $key out of $ctx, defaulting to 0. */
	static function context_id($ctx, $key) {
		if (!is_array($ctx) || !array_key_exists($key, $ctx)) {
			return 0;
		}

		if ($ctx[$key] === null) {
			return 0;
		}

		return (int)$ctx[$key];
	}

	/**
	 * page returns a page of the log, newest first, with the username and
	 * connection name resolved.
	 *
	 * A left join rather than two lookups: a row whose user or connection has
	 * since been deleted still has to render, and the id it kept is the only
	 * record that they existed.
	 */
	function page($filter, $offset, $limit) {
		$where = array("1 = 1");
		$values = array();

		if (array_key_exists("user_id", $filter) && $filter["user_id"] > 0) {
			$where[] = "a.user_id = ?";
			$values[] = (int)$filter["user_id"];
		}

		if (array_key_exists("connection_id", $filter) && $filter["connection_id"] > 0) {
			$where[] = "a.connection_id = ?";
			$values[] = (int)$filter["connection_id"];
		}

		if (array_key_exists("action", $filter) && $filter["action"] !== "") {
			$where[] = "a.action = ?";
			$values[] = $filter["action"];
		}

		if (array_key_exists("rel_table", $filter) && $filter["rel_table"] !== "") {
			$where[] = "a.rel_table = ?";
			$values[] = $filter["rel_table"];
		}

		$clause = implode(" AND ", $where);

		$total = driver_dao::run_one($this->db, "SELECT COUNT(*) AS total FROM audit a WHERE " . $clause, $values);

		$sql = "SELECT a.id, a.user_id, a.connection_id, a.rel_table, a.rel_id, a.action, a.message, a.payload," . " strftime('%Y-%m-%d %H:%M:%S', a.created_at) AS created_at," . " u.username AS username, c.name AS connection_name" . " FROM audit a" . " LEFT JOIN user u ON u.id = a.user_id" . " LEFT JOIN connection c ON c.id = a.connection_id" . " WHERE " . $clause . " ORDER BY a.id DESC LIMIT ? OFFSET ?";

		$paged = array_merge(array_copy($values), array((int)$limit, (int)$offset));

		return array(
			"rows" => driver_dao::run_all($this->db, $sql, $paged),
			"total" => (int)$total["total"],
		);
	}

	/** history returns the log for one row of one table, newest first. */
	function history($rel_table, $rel_id, $limit) {
		$sql = "SELECT a.id, a.action, a.message, strftime('%Y-%m-%d %H:%M:%S', a.created_at) AS created_at, u.username AS username" . " FROM audit a LEFT JOIN user u ON u.id = a.user_id" . " WHERE a.rel_table = ? AND a.rel_id = ?" . " ORDER BY a.id DESC LIMIT ?";

		return driver_dao::run_all($this->db, $sql, array((string)$rel_table, (string)$rel_id, (int)$limit));
	}

	/**
	 * actors returns the users and connections the log mentions, for the
	 * filter controls on the audit page.
	 *
	 * It reads the log rather than the user and connection tables, so a
	 * deleted account still appears as something the log can be filtered by.
	 */
	function actors() {
		$users = $this->db->get_all("SELECT DISTINCT a.user_id AS id, u.username AS username FROM audit a" . " LEFT JOIN user u ON u.id = a.user_id WHERE a.user_id > 0 ORDER BY u.username");
		$connections = $this->db->get_all("SELECT DISTINCT a.connection_id AS id, c.name AS name FROM audit a" . " LEFT JOIN connection c ON c.id = a.connection_id WHERE a.connection_id > 0 ORDER BY c.name");

		return array("users" => $users, "connections" => $connections);
	}
}
