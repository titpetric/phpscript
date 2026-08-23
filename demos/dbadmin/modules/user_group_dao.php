<?php

/**
 * user_group_dao stores groups, their members and the connections they reach.
 *
 * A group is the only way a non-administrator is granted a connection, and the
 * only way a policy is tightened below what the account already allows.
 */
class user_group_dao {
	public $db;
	public $audit;

	function __construct($audit) {
		$this->db = new Database("dbadmin");
		$this->audit = $audit;
	}

	/** find returns the group with id $id, or false. */
	function find($id) {
		return $this->db->get("SELECT id, name, description, destructive_policy FROM user_group WHERE id = ?", (int)$id);
	}

	/** list_all returns every group with its member and grant counts. */
	function list_all() {
		return $this->db->get_all("SELECT g.id, g.name, g.description, g.destructive_policy," . " (SELECT COUNT(*) FROM user_group_member m WHERE m.user_group_id = g.id) AS members," . " (SELECT COUNT(*) FROM user_group_connection c WHERE c.user_group_id = g.id) AS connections" . " FROM user_group g ORDER BY g.name");
	}

	/** create adds a group and returns its id. */
	function create($ctx, $name, $description, $policy) {
		$errors = user_group_dao::validate($name, $policy);
		if (count($errors) > 0) {
			throw new Exception(implode(" ", $errors), 400);
		}

		try {
			$this->db->insert("user_group", array(
				"name" => $name,
				"description" => (string)$description,
				"destructive_policy" => $policy,
			));
		} catch (Exception $e) {
			if (str_contains($e->getMessage(), "UNIQUE constraint failed")) {
				throw new Exception("A group named " . $name . " already exists.", 409);
			}

			throw $e;
		}

		$id = (int)$this->db->insert_id();

		$this->audit->log($ctx, "insert", "user_group", $id, "created group: " . $name, array("destructive_policy" => $policy));

		return $id;
	}

	/** update changes a group's name, description and policy. */
	function update($ctx, $id, $name, $description, $policy) {
		$errors = user_group_dao::validate($name, $policy);
		if (count($errors) > 0) {
			throw new Exception(implode(" ", $errors), 400);
		}

		$group = $this->find($id);
		if (!$group) {
			throw new Exception("No such group.", 404);
		}

		$this->db->query("UPDATE user_group SET name = ?, description = ?, destructive_policy = ?, updated_at = CURRENT_TIMESTAMP" . " WHERE id = ?", $name, (string)$description, $policy, (int)$id);

		$this->audit->log($ctx, "update", "user_group", $id, "updated group: " . $name, array("destructive_policy" => $policy));
	}

	/** remove deletes a group, its memberships and its grants. */
	function remove($ctx, $id) {
		$group = $this->find($id);
		if (!$group) {
			throw new Exception("No such group.", 404);
		}

		$this->db->begin();
		$this->db->query("DELETE FROM user_group_member WHERE user_group_id = ?", (int)$id);
		$this->db->query("DELETE FROM user_group_connection WHERE user_group_id = ?", (int)$id);
		$this->db->query("DELETE FROM user_group WHERE id = ?", (int)$id);
		$this->db->commit();

		$this->audit->log($ctx, "delete", "user_group", $id, "deleted group: " . $group["name"], array());
	}

	/** members returns the accounts in $group_id. */
	function members($group_id) {
		return $this->db->get_all("SELECT u.id, u.username, u.is_admin FROM user_group_member m" . " JOIN user u ON u.id = m.user_id WHERE m.user_group_id = ? ORDER BY u.username", (int)$group_id);
	}

	/** set_members replaces the membership of $group_id with $user_ids. */
	function set_members($ctx, $group_id, $user_ids) {
		$group = $this->find($group_id);
		if (!$group) {
			throw new Exception("No such group.", 404);
		}

		$this->db->begin();
		$this->db->query("DELETE FROM user_group_member WHERE user_group_id = ?", (int)$group_id);
		foreach ($user_ids as $user_id) {
			$this->db->insert("user_group_member", array("user_id" => (int)$user_id, "user_group_id" => (int)$group_id));
		}

		$this->db->commit();

		$this->audit->log($ctx, "update", "user_group", $group_id, "changed members of group: " . $group["name"], array("user_ids" => $user_ids));
	}

	/** grants returns the connections $group_id reaches, with each grant's mode. */
	function grants($group_id) {
		return $this->db->get_all("SELECT c.id, c.name, c.driver, c.is_enabled, gc.is_readonly FROM user_group_connection gc" . " JOIN connection c ON c.id = gc.connection_id WHERE gc.user_group_id = ? ORDER BY c.name", (int)$group_id);
	}

	/**
	 * set_grants replaces the grants of $group_id.
	 *
	 * $grants maps a connection id to 1 when the grant is read-only. A
	 * connection absent from the map is not granted at all, which is a
	 * different thing from being granted read-only.
	 */
	function set_grants($ctx, $group_id, $grants) {
		$group = $this->find($group_id);
		if (!$group) {
			throw new Exception("No such group.", 404);
		}

		$this->db->begin();
		$this->db->query("DELETE FROM user_group_connection WHERE user_group_id = ?", (int)$group_id);
		foreach ($grants as $connection_id => $is_readonly) {
			$this->db->insert("user_group_connection", array(
				"user_group_id" => (int)$group_id,
				"connection_id" => (int)$connection_id,
				"is_readonly" => $is_readonly ? 1 : 0,
			));
		}

		$this->db->commit();

		$this->audit->log($ctx, "update", "user_group", $group_id, "changed connections of group: " . $group["name"], array("connection_ids" => array_keys($grants)));
	}

	/** detach removes every grant of $connection_id. */
	function detach($connection_id) {
		$this->db->query("DELETE FROM user_group_connection WHERE connection_id = ?", (int)$connection_id);
	}

	/** validate returns the reasons $name and $policy are unacceptable. */
	static function validate($name, $policy) {
		$errors = array();

		if (preg_match("/^[A-Za-z0-9 _.-]{2,64}$/", (string)$name) != 1) {
			$errors[] = "A group name is 2 to 64 characters of letters, digits, space, dot, dash or underscore.";
		}

		if (!in_array($policy, user_dao::policies())) {
			$errors[] = "Unknown destructive policy.";
		}

		return $errors;
	}
}
