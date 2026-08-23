<?php

/**
 * tables_dao reads the shape of a target database.
 *
 * It is the one DAO that never opens dbadmin's own storage: every method takes
 * the client to work on, so the same code answers for a connection whichever
 * driver is behind it. That is also why it has no state — a request may hold
 * two connections open, and a DAO that remembered one of them would answer for
 * the wrong database.
 *
 * SQLite introspection uses the table-valued pragma_table_info() rather than
 * the PRAGMA statement. A read-only client refuses PRAGMA, because "PRAGMA
 * journal_mode = WAL" writes, and refusing introspection to exactly the users
 * who are only allowed to look would be the wrong half of the feature.
 */
class tables_dao {
	/**
	 * mysql_visible is the WHERE clause selecting the schemas worth listing on
	 * mysql.
	 *
	 * Only the two virtual ones are excluded. `mysql` and `sys` are real
	 * databases with real tables and a real administrator browses them, so
	 * hiding them would make the schema count on the test page disagree with
	 * the switcher beside it.
	 */
	static function mysql_visible() {
		return " WHERE schema_name NOT IN ('information_schema', 'performance_schema')";
	}

	/**
	 * pg_visible is the same clause for postgres.
	 *
	 * pg_catalog and pg_toast are the server's own, and every pg_temp_N and
	 * pg_toast_temp_N a session creates is caught by the prefix.
	 */
	static function pg_visible() {
		return " WHERE nspname NOT LIKE 'pg\\_%' AND nspname <> 'information_schema'";
	}

	/**
	 * probe runs the cheapest statement each driver answers, so a connection
	 * that is not reachable says so in the driver's own words.
	 */
	function probe($db, $driver) {
		if ($driver === "mysql") {
			return $db->get("SELECT VERSION() AS version");
		}

		if ($driver === "postgres") {
			return $db->get("SELECT version() AS version");
		}

		return $db->get("SELECT sqlite_version() AS version");
	}

	/** schemas returns the schemas the connecting user can see. */
	function schemas($db, $driver) {
		if ($driver === "mysql") {
			return array_column($db->get_all("SELECT schema_name AS name FROM information_schema.schemata" . tables_dao::mysql_visible() . " ORDER BY schema_name"), "name");
		}

		if ($driver === "postgres") {
			// pg_namespace rather than information_schema.schemata: the
			// latter lists only schemas the current role owns, so a role
			// with USAGE on a schema it did not create would not see it.
			return array_column($db->get_all("SELECT nspname AS name FROM pg_namespace" . tables_dao::pg_visible() . " ORDER BY nspname"), "name");
		}

		// sqlite has exactly one schema, and it has no name worth showing.
		return array("main");
	}

	/**
	 * tables returns the base tables of $schema, each with its column count
	 * and an estimated row count.
	 *
	 * The row count is an estimate everywhere but sqlite. A COUNT(*) per
	 * table is what makes a schema of any size slow to list, and the number
	 * on a listing page is a sense of scale rather than a fact; the browse
	 * page counts exactly, because there it is one table and the number is
	 * the pagination.
	 *
	 * The estimate is aliased row_count rather than rows, because ROWS is a
	 * reserved word in MySQL 8.0 and an unquoted alias by that name is a
	 * syntax error there and nowhere else.
	 */
	function tables($db, $driver, $schema) {
		if ($driver === "mysql") {
			return $db->get_all("SELECT t.table_name AS name," . " (SELECT COUNT(*) FROM information_schema.columns c" . "  WHERE c.table_schema = t.table_schema AND c.table_name = t.table_name) AS columns," . " t.table_rows AS row_count, t.engine AS kind" . " FROM information_schema.tables t" . " WHERE t.table_schema = ? AND t.table_type = 'BASE TABLE'" . " ORDER BY t.table_name", $schema);
		}

		if ($driver === "postgres") {
			return $db->get_all("SELECT t.table_name AS name," . " (SELECT COUNT(*) FROM information_schema.columns c" . "  WHERE c.table_schema = t.table_schema AND c.table_name = t.table_name) AS columns," . " COALESCE((SELECT cl.reltuples::bigint FROM pg_class cl" . "  JOIN pg_namespace n ON n.oid = cl.relnamespace" . "  WHERE n.nspname = t.table_schema AND cl.relname = t.table_name), 0) AS row_count," . " 'table' AS kind" . " FROM information_schema.tables t" . " WHERE t.table_schema = $1 AND t.table_type = 'BASE TABLE'" . " ORDER BY t.table_name", $schema);
		}

		return $db->get_all("SELECT m.name AS name," . " (SELECT COUNT(*) FROM pragma_table_info(m.name)) AS columns," . " 0 AS row_count, m.type AS kind" . " FROM sqlite_master m" . " WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite\\_%' ESCAPE '\\'" . " ORDER BY m.name");
	}

	/**
	 * exists reports whether $table is a base table of $schema.
	 *
	 * Every page that names a table asks this first. The name came out of a
	 * URL, and quoting it is not the same as knowing it is real.
	 */
	function exists($db, $driver, $schema, $table) {
		if (!driver_dao::identifier_ok($table)) {
			return false;
		}

		if ($driver === "sqlite") {
			$row = $db->get("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?" . " AND name NOT LIKE 'sqlite\\_%' ESCAPE '\\'", $table);
			return $row !== false;
		}

		$sql = "SELECT table_name FROM information_schema.tables" . " WHERE table_schema = " . driver_dao::placeholder($driver, 1) . " AND table_name = " . driver_dao::placeholder($driver, 2) . " AND table_type = 'BASE TABLE'";

		return driver_dao::run_one($db, $sql, array($schema, $table)) !== false;
	}

	/**
	 * columns returns the columns of $table in declaration order.
	 *
	 * The ORDER BY is not cosmetic. Rows come back from the client as maps
	 * with no column order at all, so the order a table renders in has to
	 * come from here; taking it from array_keys() of the first row gives a
	 * different layout on every request.
	 */
	function columns($db, $driver, $schema, $table) {
		if (!$this->exists($db, $driver, $schema, $table)) {
			throw new Exception("No such table: " . (string)$table, 404);
		}

		if ($driver === "sqlite") {
			$rows = $db->get_all("SELECT name, type, \"notnull\" AS not_null, dflt_value AS column_default, pk" . " FROM pragma_table_info(?) ORDER BY cid", $table);

			$columns = array();
			foreach ($rows as $row) {
				$columns[] = array(
					"name" => (string)$row["name"],
					"type" => (string)$row["type"],
					"nullable" => (int)$row["not_null"] == 0,
					"default" => $row["column_default"],
					"is_key" => (int)$row["pk"] > 0,
				);
			}

			return $columns;
		}

		$type = ($driver === "mysql") ? "column_type" : "data_type";
		$sql = "SELECT column_name AS name, " . $type . " AS type, is_nullable, column_default" . " FROM information_schema.columns" . " WHERE table_schema = " . driver_dao::placeholder($driver, 1) . " AND table_name = " . driver_dao::placeholder($driver, 2) . " ORDER BY ordinal_position";

		$keys = $this->key_columns($db, $driver, $schema, $table);
		$rows = driver_dao::run_all($db, $sql, array($schema, $table));

		$columns = array();
		foreach ($rows as $row) {
			$columns[] = array(
				"name" => (string)$row["name"],
				"type" => (string)$row["type"],
				"nullable" => strtoupper((string)$row["is_nullable"]) === "YES",
				"default" => $row["column_default"],
				"is_key" => in_array((string)$row["name"], $keys),
			);
		}

		return $columns;
	}

	/** indexes returns the indexes on $table, one row per index. */
	function indexes($db, $driver, $schema, $table) {
		if ($driver === "sqlite") {
			return $db->get_all("SELECT il.name AS name, il.\"unique\" AS is_unique," . " (SELECT group_concat(ii.name, ', ') FROM pragma_index_info(il.name) ii) AS columns" . " FROM pragma_index_list(?) il ORDER BY il.name", $table);
		}

		if ($driver === "mysql") {
			return $db->get_all("SELECT index_name AS name, MIN(non_unique) = 0 AS is_unique," . " GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ', ') AS columns" . " FROM information_schema.statistics WHERE table_schema = ? AND table_name = ?" . " GROUP BY index_name ORDER BY index_name", $schema, $table);
		}

		return driver_dao::run_all($db, "SELECT indexname AS name, indexdef AS columns," . " (indexdef LIKE 'CREATE UNIQUE%') AS is_unique" . " FROM pg_indexes WHERE schemaname = $1 AND tablename = $2 ORDER BY indexname", array($schema, $table));
	}

	/**
	 * key_columns returns the primary key columns of $table, in key order.
	 *
	 * An empty result means the table has no primary key. On sqlite that is
	 * survivable, because there is a rowid to fall back on; on the other two
	 * it means a row cannot be addressed, and editing and deleting are off.
	 */
	function key_columns($db, $driver, $schema, $table) {
		if ($driver === "sqlite") {
			return array_column($db->get_all("SELECT name FROM pragma_table_info(?) WHERE pk > 0 ORDER BY pk", $table), "name");
		}

		$sql = "SELECT kcu.column_name AS name" . " FROM information_schema.table_constraints tc" . " JOIN information_schema.key_column_usage kcu" . "  ON kcu.constraint_name = tc.constraint_name" . "  AND kcu.table_schema = tc.table_schema" . "  AND kcu.table_name = tc.table_name" . " WHERE tc.constraint_type = 'PRIMARY KEY'" . " AND tc.table_schema = " . driver_dao::placeholder($driver, 1) . " AND tc.table_name = " . driver_dao::placeholder($driver, 2) . " ORDER BY kcu.ordinal_position";

		return array_column(driver_dao::run_all($db, $sql, array($schema, $table)), "name");
	}

	/**
	 * identity returns how a single row of $table is addressed.
	 *
	 * kind is "key" when there is a primary key, "rowid" when sqlite's
	 * implicit one is available, and "none" when a row cannot be named. The
	 * last case is not an error: browse and insert still work, and only edit
	 * and delete are withdrawn, which is what phpMyAdmin does too.
	 */
	function identity($db, $driver, $schema, $table) {
		$keys = $this->key_columns($db, $driver, $schema, $table);
		if (count($keys) > 0) {
			return array("kind" => "key", "columns" => $keys);
		}

		if ($driver === "sqlite" && !$this->without_rowid($db, $table)) {
			return array("kind" => "rowid", "columns" => array("rowid"));
		}

		return array("kind" => "none", "columns" => array());
	}

	/** without_rowid reports whether $table was declared WITHOUT ROWID. */
	function without_rowid($db, $table) {
		$row = $db->get("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", $table);
		if (!$row) {
			return true;
		}

		return str_contains(strtoupper((string)$row["sql"]), "WITHOUT ROWID");
	}

	/** definition returns the CREATE statement of $table where the driver keeps one. */
	function definition($db, $driver, $table) {
		if ($driver === "sqlite") {
			$row = $db->get("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", $table);
			return $row ? (string)$row["sql"] : "";
		}

		if ($driver === "mysql") {
			$row = $db->get("SHOW CREATE TABLE " . driver_dao::quote_ident($driver, $table));
			if (!$row) {
				return "";
			}

			// SHOW CREATE TABLE names its second column "Create Table",
			// which is awkward to index and is the only column that
			// matters, so it is found rather than named.
			foreach ($row as $key => $value) {
				if (str_contains(strtolower((string)$key), "create")) {
					return (string)$value;
				}
			}

			return "";
		}

		// postgres has no server-side DDL dump; the structure page renders
		// the column list instead, which is what a reader wanted anyway.
		return "";
	}

	/**
	 * fill_row_counts replaces the estimated row counts in $listing with
	 * exact ones.
	 *
	 * sqlite has no row estimate to report, so its listing arrives with
	 * zeroes and this is what fills them. It is one COUNT(*) per table, which
	 * is what makes phpMyAdmin slow on a large schema, so it is capped: past
	 * the cap the numbers stay zero and the page says the count is not shown
	 * rather than taking a second to draw.
	 */
	function fill_row_counts($db, $driver, $schema, $listing, $cap) {
		if (count($listing) > $cap) {
			return array("rows" => $listing, "counted" => false);
		}

		$counted = array();
		foreach ($listing as $entry) {
			$row = array_copy($entry);
			$row["row_count"] = $this->row_count($db, $driver, $schema, $entry["name"]);
			$counted[] = $row;
		}

		return array("rows" => $counted, "counted" => true);
	}

	/** row_count returns the exact number of rows in $table. */
	function row_count($db, $driver, $schema, $table) {
		$row = $db->get("SELECT COUNT(*) AS total FROM " . driver_dao::qualify($driver, $schema, $table));
		return (int)$row["total"];
	}

	/**
	 * counts returns the table, column and schema totals the test page shows.
	 *
	 * sqlite reports one schema by definition. The other two report what the
	 * connecting user can see, which is the number that matters: a server
	 * with forty schemas the role cannot enter has none as far as this
	 * connection is concerned.
	 */
	function counts($db, $driver, $schema) {
		if ($driver === "mysql") {
			$tables = $db->get("SELECT COUNT(*) AS total FROM information_schema.tables" . " WHERE table_schema = ? AND table_type = 'BASE TABLE'", $schema);
			$columns = $db->get("SELECT COUNT(*) AS total FROM information_schema.columns WHERE table_schema = ?", $schema);
			$schemas = $db->get("SELECT COUNT(*) AS total FROM information_schema.schemata" . tables_dao::mysql_visible());

			return tables_dao::totals($tables, $columns, $schemas);
		}

		if ($driver === "postgres") {
			$tables = driver_dao::run_one($db, "SELECT COUNT(*) AS total FROM information_schema.tables" . " WHERE table_schema = $1 AND table_type = 'BASE TABLE'", array($schema));
			$columns = driver_dao::run_one($db, "SELECT COUNT(*) AS total FROM information_schema.columns" . " WHERE table_schema = $1", array($schema));
			$schemas = $db->get("SELECT COUNT(*) AS total FROM pg_namespace" . tables_dao::pg_visible());

			return tables_dao::totals($tables, $columns, $schemas);
		}

		$tables = $db->get("SELECT COUNT(*) AS total FROM sqlite_master" . " WHERE type = 'table' AND name NOT LIKE 'sqlite\\_%' ESCAPE '\\'");
		$columns = $db->get("SELECT COUNT(*) AS total FROM sqlite_master m, pragma_table_info(m.name) p" . " WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite\\_%' ESCAPE '\\'");

		return array(
			"tables" => (int)$tables["total"],
			"columns" => (int)$columns["total"],
			"schemas" => 1,
		);
	}

	/** totals reads the three count rows into one array. */
	static function totals($tables, $columns, $schemas) {
		return array(
			"tables" => $tables ? (int)$tables["total"] : 0,
			"columns" => $columns ? (int)$columns["total"] : 0,
			"schemas" => $schemas ? (int)$schemas["total"] : 0,
		);
	}
}
