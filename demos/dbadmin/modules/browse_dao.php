<?php

/**
 * browse_dao reads rows out of a target table.
 *
 * The select list is built from the introspected columns rather than written
 * as "*", for three reasons: rows arrive as maps with no column order, so the
 * order has to come from somewhere; a BLOB has no printable form and there is
 * no base64_encode() here, so it is hex-encoded by the server; and a timestamp
 * has no date() to format it with, so it is formatted by the server too.
 */
class browse_dao {
	/** PAGE_SIZE is how many rows a browse page shows. */
	const PAGE_SIZE = 30;

	public $tables;
	public $audit;

	function __construct($tables, $audit) {
		$this->tables = $tables;
		$this->audit = $audit;
	}

	/**
	 * page returns one page of $table, with the columns and row identity the
	 * template needs to render and link it.
	 */
	function page($db, $driver, $schema, $table, $search, $page) {
		$columns = $this->tables->columns($db, $driver, $schema, $table);
		$identity = $this->tables->identity($db, $driver, $schema, $table);
		$qualified = driver_dao::qualify($driver, $schema, $table);

		$where = "";
		$values = array();
		if ($search !== "") {
			$clauses = array();
			foreach ($columns as $column) {
				$clauses[] = driver_dao::text_expr($driver, $column["name"]) . " LIKE " . driver_dao::placeholder($driver, count($values) + 1);
				$values[] = "%" . $search . "%";
			}

			if (count($clauses) > 0) {
				$where = " WHERE (" . implode(" OR ", $clauses) . ")";
			}
		}

		$total_row = driver_dao::run_one($db, "SELECT COUNT(*) AS total FROM " . $qualified . $where, $values);
		$total = (int)$total_row["total"];

		$pages = max2(1, div_ceil($total, browse_dao::PAGE_SIZE));
		$page = max2(1, min2($page, $pages));
		$offset = ($page - 1) * browse_dao::PAGE_SIZE;

		$select = browse_dao::select_list($driver, $columns, $identity);
		$order = browse_dao::order_by($driver, $identity, $columns);

		$limit_at = count($values) + 1;
		$sql = "SELECT " . $select . " FROM " . $qualified . $where . $order . " LIMIT " . driver_dao::placeholder($driver, $limit_at) . " OFFSET " . driver_dao::placeholder($driver, $limit_at + 1);

		$paged = array_merge(array_copy($values), array(browse_dao::PAGE_SIZE, $offset));

		return array(
			"columns" => $columns,
			"identity" => $identity,
			"rows" => driver_dao::run_all($db, $sql, $paged),
			"total" => $total,
			"page" => $page,
			"pages" => $pages,
			"page_size" => browse_dao::PAGE_SIZE,
			"search" => $search,
		);
	}

	/**
	 * select_list renders each column under its own name, encoding the ones
	 * that have no readable form of their own.
	 */
	static function select_list($driver, $columns, $identity) {
		$parts = array();

		if ($identity["kind"] === "rowid") {
			$parts[] = "rowid AS __key";
		}

		foreach ($columns as $column) {
			$name = $column["name"];
			$quoted = driver_dao::quote_ident($driver, $name);
			$alias = " AS " . $quoted;

			if (browse_dao::is_binary($column["type"])) {
				$parts[] = driver_dao::hex_expr($driver, $name) . $alias;
			} elseif (browse_dao::is_temporal($column["type"])) {
				$parts[] = driver_dao::date_expr($driver, $name) . $alias;
			} else {
				$parts[] = $quoted;
			}
		}

		return implode(", ", $parts);
	}

	/**
	 * order_by returns a stable ordering.
	 *
	 * A page of rows with no ORDER BY is whatever the engine felt like, and
	 * page two of an unordered result can repeat page one. The primary key
	 * is the ordering when there is one, the rowid when there is not, and the
	 * first column as a last resort, which is at least deterministic.
	 */
	static function order_by($driver, $identity, $columns) {
		if ($identity["kind"] === "rowid") {
			return " ORDER BY rowid";
		}

		if ($identity["kind"] === "key") {
			$parts = array();
			foreach ($identity["columns"] as $column) {
				$parts[] = driver_dao::quote_ident($driver, $column);
			}

			return " ORDER BY " . implode(", ", $parts);
		}

		if (count($columns) > 0) {
			return " ORDER BY " . driver_dao::quote_ident($driver, $columns[0]["name"]);
		}

		return "";
	}

	/**
	 * find returns one row of $table addressed by $key.
	 *
	 * $key is the "__key" value the browse page rendered: the rowid, or the
	 * primary key columns joined by a separator no identifier can contain.
	 */
	function find($db, $driver, $schema, $table, $key) {
		$columns = $this->tables->columns($db, $driver, $schema, $table);
		$identity = $this->tables->identity($db, $driver, $schema, $table);
		if ($identity["kind"] === "none") {
			throw new Exception("This table has no primary key, so a single row cannot be addressed.", 409);
		}

		$where = browse_dao::key_where($driver, $identity, $key, 1);
		$sql = "SELECT " . browse_dao::select_list($driver, $columns, $identity) . " FROM " . driver_dao::qualify($driver, $schema, $table) . " WHERE " . $where["sql"] . " LIMIT 1";

		$row = driver_dao::run_one($db, $sql, $where["values"]);
		if (!$row) {
			throw new Exception("No such row.", 404);
		}

		return array(
			"columns" => $columns,
			"identity" => $identity,
			"row" => $row,
			"key" => $key,
		);
	}

	/**
	 * key_where turns a rendered key back into a WHERE clause.
	 *
	 * Composite keys are joined with a byte no SQL identifier and no sensible
	 * key value contains. A value that does contain it cannot be addressed,
	 * which is a limit worth having in the open rather than a URL format that
	 * needs escaping rules of its own.
	 */
	static function key_where($driver, $identity, $key, $first) {
		if ($identity["kind"] === "rowid") {
			return array(
				"sql" => "rowid = " . driver_dao::placeholder($driver, $first),
				"values" => array((int)$key),
			);
		}

		$parts = explode("~", (string)$key);
		if (count($parts) != count($identity["columns"])) {
			throw new Exception("Malformed row key.", 400);
		}

		$clauses = array();
		$values = array();
		foreach ($identity["columns"] as $index => $column) {
			$clauses[] = driver_dao::quote_ident($driver, $column) . " = " . driver_dao::placeholder($driver, $first + $index);
			$values[] = $parts[$index];
		}

		return array("sql" => implode(" AND ", $clauses), "values" => $values);
	}

	/** key_of renders the addressable key of $row, or "" when there is none. */
	static function key_of($identity, $row) {
		if ($identity["kind"] === "rowid") {
			return (string)$row["__key"];
		}

		if ($identity["kind"] === "none") {
			return "";
		}

		$parts = array();
		foreach ($identity["columns"] as $column) {
			$parts[] = (string)$row[$column];
		}

		return implode("~", $parts);
	}

	/**
	 * export writes $table to the output as CSV.
	 *
	 * Rows are streamed a page at a time rather than collected: an export is
	 * the one page that has no upper bound on how much it touches, and
	 * holding a whole table in memory to print it is the wrong trade.
	 */
	function export($db, $driver, $schema, $table, $ctx) {
		$columns = $this->tables->columns($db, $driver, $schema, $table);
		$identity = $this->tables->identity($db, $driver, $schema, $table);
		$qualified = driver_dao::qualify($driver, $schema, $table);

		$names = array();
		foreach ($columns as $column) {
			$names[] = browse_dao::csv_cell($column["name"]);
		}

		echo implode(",", $names), "\r\n";

		$select = browse_dao::select_list($driver, $columns, $identity);
		$order = browse_dao::order_by($driver, $identity, $columns);
		$offset = 0;
		$batch = 500;

		while (true) {
			$sql = "SELECT " . $select . " FROM " . $qualified . $order . " LIMIT " . driver_dao::placeholder($driver, 1) . " OFFSET " . driver_dao::placeholder($driver, 2);
			$rows = driver_dao::run_all($db, $sql, array($batch, $offset));
			if (count($rows) == 0) {
				break;
			}

			foreach ($rows as $row) {
				$cells = array();
				foreach ($columns as $column) {
					$cells[] = browse_dao::csv_cell($row[$column["name"]]);
				}

				echo implode(",", $cells), "\r\n";
			}

			$offset = $offset + $batch;
			if (count($rows) < $batch) {
				break;
			}
		}

		$this->audit->log($ctx, "select", $table, "", "exported table: " . $table, array("schema" => $schema));
	}

	/** csv_cell quotes $value for a CSV field. */
	static function csv_cell($value) {
		if ($value === null) {
			return "";
		}

		return "\"" . str_replace("\"", "\"\"", (string)$value) . "\"";
	}

	/** is_binary reports whether a column of $type holds bytes. */
	static function is_binary($type) {
		$type = strtolower((string)$type);
		return str_contains($type, "blob") || str_contains($type, "bytea") || str_contains($type, "binary");
	}

	/** is_temporal reports whether a column of $type holds a point in time. */
	static function is_temporal($type) {
		$type = strtolower((string)$type);
		if (str_contains($type, "timestamp") || str_contains($type, "datetime")) {
			return true;
		}

		// "date" alone matches, but not "update" or a user type whose name
		// merely contains it, so the match is anchored.
		return $type === "date" || str_starts_with($type, "date ") || str_starts_with($type, "date(");
	}
}
