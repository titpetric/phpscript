<?php

/**
 * acl_dao answers what the logged-in user may reach and may do.
 *
 * Two rules, resolved in opposite directions. Access is the loosest grant among
 * a user's groups, because being in a second group should not take a
 * connection away. Destructive policy is the strictest, because a group is a
 * way to tighten an account and never a way to loosen one.
 */
class acl_dao {
	public $db;
	public $groups;
	public $connections;

	function __construct($groups, $connections) {
		$this->db = new Database("dbadmin");
		$this->groups = $groups;
		$this->connections = $connections;
	}

	/**
	 * connections_for returns the connections $ctx may open.
	 *
	 * An administrator bypasses the grant join: they administer the list, so
	 * a list they cannot see would be one they cannot fix.
	 */
	function connections_for($ctx) {
		if ($ctx["is_admin"]) {
			return $this->db->get_all("SELECT id, name, driver, status, status_message, default_schema, table_count," . " is_readonly AS connection_readonly, 0 AS grant_readonly" . " FROM connection WHERE is_enabled = 1 ORDER BY name");
		}

		return $this->db->get_all("SELECT c.id, c.name, c.driver, c.status, c.status_message, c.default_schema, c.table_count," . " c.is_readonly AS connection_readonly, MIN(gc.is_readonly) AS grant_readonly" . " FROM connection c" . " JOIN user_group_connection gc ON gc.connection_id = c.id" . " JOIN user_group_member m ON m.user_group_id = gc.user_group_id" . " WHERE m.user_id = ? AND c.is_enabled = 1" . " GROUP BY c.id, c.name, c.driver, c.status, c.status_message, c.default_schema, c.table_count, c.is_readonly" . " ORDER BY c.name", (int)$ctx["user_id"]);
	}

	/**
	 * may_use reports whether $ctx may open $connection_id, and in what mode.
	 *
	 * The returned mode is the connection's own read-only flag or the
	 * grant's, whichever is set: a read-only connection cannot be written
	 * through a read-write grant.
	 */
	function may_use($ctx, $connection_id) {
		if ($connection_id == 0) {
			return array("allowed" => false, "is_readonly" => true);
		}

		if ($ctx["is_admin"]) {
			$row = $this->db->get("SELECT is_readonly FROM connection WHERE id = ? AND is_enabled = 1", (int)$connection_id);
			if (!$row) {
				return array("allowed" => false, "is_readonly" => true);
			}

			return array("allowed" => true, "is_readonly" => (int)$row["is_readonly"] == 1);
		}

		$row = $this->db->get("SELECT MIN(gc.is_readonly) AS grant_readonly, c.is_readonly AS connection_readonly" . " FROM connection c" . " JOIN user_group_connection gc ON gc.connection_id = c.id" . " JOIN user_group_member m ON m.user_group_id = gc.user_group_id" . " WHERE m.user_id = ? AND c.id = ? AND c.is_enabled = 1" . " GROUP BY c.id, c.is_readonly", (int)$ctx["user_id"], (int)$connection_id);

		if (!$row) {
			return array("allowed" => false, "is_readonly" => true);
		}

		$readonly = (int)$row["grant_readonly"] == 1 || (int)$row["connection_readonly"] == 1;
		return array("allowed" => true, "is_readonly" => $readonly);
	}

	/**
	 * policy returns the effective destructive policy for $ctx.
	 *
	 * An administrator is held to their own account setting and to nothing
	 * else. They can already change any group they are in, so letting a group
	 * restrict them would be a lock with the key taped to it.
	 */
	function policy($ctx) {
		$own = (string)$ctx["destructive_policy"];
		if ($ctx["is_admin"]) {
			return $own;
		}

		$rows = $this->db->get_all("SELECT g.destructive_policy FROM user_group_member m" . " JOIN user_group g ON g.id = m.user_group_id WHERE m.user_id = ?", (int)$ctx["user_id"]);

		$effective = $own;
		foreach ($rows as $row) {
			$effective = acl_dao::stricter($effective, (string)$row["destructive_policy"]);
		}

		return $effective;
	}

	/**
	 * stricter returns whichever of $a and $b permits less.
	 *
	 * Ranked denied, toggle, allowed. An unrecognised value ranks as denied,
	 * so a policy nobody understands is a policy that permits nothing.
	 */
	static function stricter($a, $b) {
		return (acl_dao::rank($a) < acl_dao::rank($b)) ? $a : $b;
	}

	/** rank scores a policy, lower being stricter. */
	static function rank($policy) {
		if ($policy === "allowed") {
			return 2;
		}

		if ($policy === "toggle") {
			return 1;
		}

		return 0;
	}

	/**
	 * decide returns the destructive permissions of $ctx as three flags the
	 * templates and the guards both read.
	 *
	 * offers_toggle is what decides whether the switch is drawn; can_destroy
	 * is what decides whether a statement runs. They are different questions
	 * and a page that conflates them either hides a working control or draws
	 * one that does nothing.
	 */
	function decide($ctx) {
		$policy = $this->policy($ctx);

		if ($policy === "allowed") {
			return array(
				"policy" => $policy,
				"offers_toggle" => false,
				"can_destroy" => true,
			);
		}

		if ($policy === "toggle") {
			return array(
				"policy" => $policy,
				"offers_toggle" => true,
				"can_destroy" => $ctx["is_destructive"],
			);
		}

		return array(
			"policy" => $policy,
			"offers_toggle" => false,
			"can_destroy" => false,
		);
	}
}
