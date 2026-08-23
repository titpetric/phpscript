<?php

/**
 * ddl_dao creates, empties and drops tables.
 *
 * Every method here is destructive except create(), so every method here takes
 * the session's decision as an argument and refuses without it. The refusal is
 * audited: an attempt that was stopped is worth more in the log than a
 * successful drop, because it is the one that says somebody tried.
 */
class ddl_dao {
	public $tables;
	public $audit;

	function __construct($tables, $audit) {
		$this->tables = $tables;
		$this->audit = $audit;
	}

	/**
	 * default_type is what a column the user has not given a type gets.
	 *
	 * It is the driver's string type, not the first entry of types(). A form
	 * whose every dropdown starts on INTEGER hands somebody who filled in
	 * three column names and left the dropdowns alone three integer columns,
	 * which is not what they asked for and is not what they will notice until
	 * the first insert.
	 */
	static function default_type($driver) {
		if ($driver === "sqlite") {
			return "TEXT";
		}

		return "VARCHAR(255)";
	}

	/** TYPES lists the column types the create form offers, per driver. */
	static function types($driver) {
		if ($driver === "mysql") {
			return array(
				"INT",
				"BIGINT",
				"VARCHAR(255)",
				"TEXT",
				"DECIMAL(10,2)",
				"DATETIME",
				"DATE",
				"TINYINT(1)",
				"JSON",
				"BLOB",
			);
		}

		if ($driver === "postgres") {
			return array(
				"INTEGER",
				"BIGINT",
				"VARCHAR(255)",
				"TEXT",
				"NUMERIC(10,2)",
				"TIMESTAMP",
				"DATE",
				"BOOLEAN",
				"JSONB",
				"BYTEA",
			);
		}

		return array("INTEGER", "TEXT", "REAL", "NUMERIC", "BLOB", "DATETIME", "BOOLEAN");
	}

	/**
	 * create builds a table from $spec, a list of column definitions.
	 *
	 * Each entry has a name, a type chosen from types(), and the not_null,
	 * primary and autoincrement flags. Types are chosen from a list rather
	 * than typed, because a type is not something that can be bound as a
	 * parameter and validating free text well enough to concatenate it is a
	 * larger job than offering the ten types anybody asks for.
	 */
	function create($db, $driver, $schema, $table, $spec, $ctx) {
		if (!driver_dao::identifier_ok($table)) {
			throw new Exception("A table name starts with a letter or underscore and continues with letters, digits or underscores.", 400);
		}

		if ($this->tables->exists($db, $driver, $schema, $table)) {
			throw new Exception("A table named " . $table . " already exists.", 409);
		}

		$allowed = ddl_dao::types($driver);
		$parts = array();
		$keys = array();

		foreach ($spec as $column) {
			$name = trim((string)$column["name"]);
			if ($name === "") {
				continue;
			}

			if (!driver_dao::identifier_ok($name)) {
				throw new Exception("Invalid column name: " . $name, 400);
			}

			if (!in_array($column["type"], $allowed)) {
				throw new Exception("Unsupported column type: " . (string)$column["type"], 400);
			}

			$definition = driver_dao::quote_ident($driver, $name) . " " . $column["type"];

			if ($column["autoincrement"]) {
				$definition = ddl_dao::autoincrement($driver, $name);
				$parts[] = $definition;
				if ($driver !== "sqlite") {
					$keys[] = driver_dao::quote_ident($driver, $name);
				}

				continue;
			}

			if ($column["not_null"]) {
				$definition = $definition . " NOT NULL";
			}

			if ($column["primary"]) {
				$keys[] = driver_dao::quote_ident($driver, $name);
			}

			$parts[] = $definition;
		}

		if (count($parts) == 0) {
			throw new Exception("A table needs at least one column.", 400);
		}

		if (count($keys) > 0) {
			$parts[] = "PRIMARY KEY (" . implode(", ", $keys) . ")";
		}

		$sql = "CREATE TABLE " . driver_dao::qualify($driver, $schema, $table) . " (" . implode(", ", $parts) . ")";

		$db->query($sql);

		$this->audit->log($ctx, "create", $table, "", "created table: " . $table, array("schema" => $schema, "sql" => $sql));

		return $sql;
	}

	/**
	 * autoincrement returns the column definition for a generated key.
	 *
	 * The three spellings are mutually exclusive, which is the whole reason
	 * this is a method rather than a string in the caller.
	 */
	static function autoincrement($driver, $name) {
		$quoted = driver_dao::quote_ident($driver, $name);
		if ($driver === "mysql") {
			return $quoted . " BIGINT NOT NULL AUTO_INCREMENT";
		}

		if ($driver === "postgres") {
			return $quoted . " BIGSERIAL NOT NULL";
		}

		return $quoted . " INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL";
	}

	/**
	 * truncate removes every row of $table without removing the table.
	 *
	 * sqlite has no TRUNCATE. DELETE without a WHERE is what it optimises
	 * into the same thing, so that is what it gets.
	 */
	function truncate($db, $driver, $schema, $table, $can_destroy, $ctx) {
		$this->require_destroy($can_destroy, $ctx, "truncate", $table, $schema);

		if (!$this->tables->exists($db, $driver, $schema, $table)) {
			throw new Exception("No such table: " . (string)$table, 404);
		}

		$qualified = driver_dao::qualify($driver, $schema, $table);
		$before = $this->tables->row_count($db, $driver, $schema, $table);

		if ($driver === "sqlite") {
			$db->query("DELETE FROM " . $qualified);
		} else {
			$db->query("TRUNCATE TABLE " . $qualified);
		}

		$this->audit->log($ctx, "truncate", $table, "", "emptied table: " . $table, array("schema" => $schema, "rows" => $before));

		return $before;
	}

	/** drop removes $table and everything in it. */
	function drop($db, $driver, $schema, $table, $can_destroy, $ctx) {
		$this->require_destroy($can_destroy, $ctx, "drop", $table, $schema);

		if (!$this->tables->exists($db, $driver, $schema, $table)) {
			throw new Exception("No such table: " . (string)$table, 404);
		}

		$before = $this->tables->row_count($db, $driver, $schema, $table);

		$db->query("DROP TABLE " . driver_dao::qualify($driver, $schema, $table));

		$this->audit->log($ctx, "drop", $table, "", "dropped table: " . $table, array("schema" => $schema, "rows" => $before));

		return $before;
	}

	/** require_destroy refuses and records an attempt made without permission. */
	function require_destroy($can_destroy, $ctx, $action, $table, $schema) {
		if ($can_destroy) {
			return;
		}

		$this->audit->log($ctx, "denied", $table, "", "refused to " . $action . " table: " . $table, array("schema" => $schema));
		throw new Exception("Destructive actions are not enabled for this session.", 403);
	}
}
