<?php

/**
 * sql_dao runs what a user types into the console.
 *
 * The console is the one page where the statement is not assembled by dbadmin,
 * so it is the one page that has to read a statement to decide what it is.
 * classify() does that on the leading keyword and nothing else, which is the
 * same rule the read-only client applies underneath; agreeing with it means a
 * refusal is explained here rather than surfacing as a driver error.
 */
class sql_dao {
	/** MAX_ROWS is how many rows the console will render. */
	const MAX_ROWS = 200;

	public $audit;

	function __construct($audit) {
		$this->audit = $audit;
	}

	/** READS are the verbs that return rows and change nothing. */
	static function reads() {
		return array("select", "show", "describe", "desc", "explain", "with", "values", "table");
	}

	/** DESTRUCTIVE are the verbs that lose data or structure. */
	static function destructive() {
		return array("delete", "drop", "truncate", "alter", "rename");
	}

	/**
	 * classify reports what a statement is: read, write, destructive, or
	 * empty.
	 *
	 * A WITH is called out separately by run(). It reads, but the client's
	 * read-only allowlist does not include it, so a read-only session sending
	 * a CTE would otherwise be told that "with is not allowed" without being
	 * told why.
	 */
	static function classify($sql) {
		$verb = driver_dao::verb($sql);
		if ($verb === "") {
			return "empty";
		}

		if (in_array($verb, sql_dao::destructive())) {
			return "destructive";
		}

		if (in_array($verb, sql_dao::reads())) {
			return "read";
		}

		return "write";
	}

	/**
	 * run executes $sql and returns a message, the rows it produced and the
	 * order to render their columns in.
	 *
	 * Rows are maps with no column order, so the console sorts the names.
	 * Alphabetical is not the order the statement asked for, but it is the
	 * same order on every request, which a table that redraws itself
	 * differently each time is not.
	 */
	function run($db, $driver, $sql, $can_destroy, $readonly, $ctx) {
		$sql = trim((string)$sql);
		$kind = sql_dao::classify($sql);

		if ($kind === "empty") {
			throw new Exception("Nothing to run.", 400);
		}

		if ($kind === "destructive" && !$can_destroy) {
			$this->audit->log($ctx, "denied", "", "", "refused a destructive statement", array("sql" => $sql));
			throw new Exception("This statement is destructive and destructive actions are not enabled for this session.", 403);
		}

		if ($kind !== "read" && $readonly) {
			throw new Exception("This connection is read-only for you.", 403);
		}

		if ($readonly && str_starts_with(strtolower($sql), "with")) {
			throw new Exception("A read-only connection accepts SELECT, SHOW and DESCRIBE. A WITH clause is a read, but the" . " connection cannot tell that from its first word; run it as a SELECT.", 400);
		}

		if ($kind !== "read") {
			$db->query($sql);
			$affected = (int)$db->rows_affected();

			$this->audit->log_statement($ctx, $sql, "", "", "ran a statement in the console", array("sql" => $sql, "rows" => $affected));

			return array(
				"message" => "Statement executed. " . (string)$affected . " row(s) affected.",
				"rows" => array(),
				"columns" => array(),
				"truncated" => false,
			);
		}

		$rows = $db->get_all($sql);

		$this->audit->log($ctx, "select", "", "", "ran a query in the console", array("sql" => $sql, "rows" => count($rows)));

		$columns = array();
		if (count($rows) > 0) {
			$columns = array_keys($rows[0]);

			sort($columns);
		}

		$truncated = count($rows) > sql_dao::MAX_ROWS;
		if ($truncated) {
			$rows = array_slice($rows, 0, sql_dao::MAX_ROWS);
		}

		$message = "Query completed. " . (string)count($rows) . " row(s) shown.";
		if ($truncated) {
			$message = "Query completed. First " . (string)sql_dao::MAX_ROWS . " rows shown; there are more.";
		}

		return array(
			"message" => $message,
			"rows" => $rows,
			"columns" => $columns,
			"truncated" => $truncated,
		);
	}
}
