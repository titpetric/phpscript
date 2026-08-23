<?php

/**
 * driver_dao writes the dialect differences between sqlite, mysql and postgres.
 *
 * It is the only file that knows a driver name means anything. Every statement
 * dbadmin sends to a target database is assembled through these helpers, so a
 * dialect bug has one place to be fixed and one place to be read.
 *
 * The methods are static and there is no controller beside this file: it holds
 * no state and answers no route, so a sidecar of its own would be empty.
 */
class driver_dao {
	/** DRIVERS is every driver a connection may name. */
	const DRIVERS = array("sqlite", "mysql", "postgres");

	/**
	 * identifier_ok reports whether $name is a plain SQL identifier.
	 *
	 * Table and column names arrive from the URL and from forms, and they
	 * cannot be bound as parameters. Everything that reaches quote_ident()
	 * passes this first, so the quoting below is a second line rather than
	 * the only one.
	 */
	static function identifier_ok($name) {
		if ($name === null || $name === "") {
			return false;
		}

		return preg_match("/^[A-Za-z_][A-Za-z0-9_]*$/", (string)$name) == 1;
	}

	/** quote_ident returns $name quoted for $driver. */
	static function quote_ident($driver, $name) {
		if (!driver_dao::identifier_ok($name)) {
			throw new Exception("invalid SQL identifier: " . (string)$name, 400);
		}

		if ($driver === "mysql") {
			return "`" . $name . "`";
		}

		return "\"" . $name . "\"";
	}

	/**
	 * placeholder returns the bind marker for the $position-th argument,
	 * counting from one.
	 *
	 * postgres numbers its placeholders and rejects a question mark. The
	 * driver rebinds a statement it is given arguments for, but a statement
	 * this application assembles says what it means: the generated SQL and
	 * the SQL a user pastes into the console then read the same way.
	 */
	static function placeholder($driver, $position) {
		if ($driver === "postgres") {
			return "$" . (string)$position;
		}

		return "?";
	}

	/**
	 * placeholders returns $count bind markers, comma separated, starting at
	 * $first.
	 */
	static function placeholders($driver, $count, $first) {
		$marks = array();
		for ($i = 0; $i < $count; $i += 1) {
			$marks[] = driver_dao::placeholder($driver, $first + $i);
		}

		return implode(", ", $marks);
	}

	/**
	 * qualify returns $table quoted and, where the driver has schemas,
	 * prefixed with $schema.
	 *
	 * sqlite has one schema and mysql selects its database in the DSN, so
	 * only postgres qualifies. Qualifying on mysql would work but would put
	 * the database name into every statement in the query log for no gain.
	 */
	static function qualify($driver, $schema, $table) {
		if ($driver === "postgres" && $schema !== "") {
			return driver_dao::quote_ident($driver, $schema) . "." . driver_dao::quote_ident($driver, $table);
		}

		return driver_dao::quote_ident($driver, $table);
	}

	/**
	 * hex_expr returns an expression rendering $column as hexadecimal.
	 *
	 * A BLOB has no printable form and there is no base64_encode() in this
	 * runtime, so the encoding happens on the server that owns the bytes.
	 */
	static function hex_expr($driver, $column) {
		$quoted = driver_dao::quote_ident($driver, $column);
		if ($driver === "mysql") {
			return "HEX(" . $quoted . ")";
		}

		if ($driver === "postgres") {
			return "encode(" . $quoted . "::bytea, 'hex')";
		}

		return "hex(" . $quoted . ")";
	}

	/**
	 * date_expr returns an expression rendering $column as a readable
	 * timestamp.
	 *
	 * There is no date() in this runtime. Formatting on the server also
	 * means the value is formatted in the timezone the data lives in.
	 */
	static function date_expr($driver, $column) {
		$quoted = driver_dao::quote_ident($driver, $column);
		if ($driver === "mysql") {
			return "DATE_FORMAT(" . $quoted . ", '%Y-%m-%d %H:%i:%s')";
		}

		if ($driver === "postgres") {
			return "to_char(" . $quoted . ", 'YYYY-MM-DD HH24:MI:SS')";
		}

		return "strftime('%Y-%m-%d %H:%M:%S', " . $quoted . ")";
	}

	/** text_expr returns $column cast to text, for a LIKE search. */
	static function text_expr($driver, $column) {
		$quoted = driver_dao::quote_ident($driver, $column);
		if ($driver === "mysql") {
			return "CAST(" . $quoted . " AS CHAR)";
		}

		return "CAST(" . $quoted . " AS TEXT)";
	}

	/**
	 * driver_of returns the driver named by the "<driver>://" prefix of
	 * $dsn, or "" when it names none.
	 */
	static function driver_of($dsn) {
		$at = strpos((string)$dsn, "://");
		if ($at === false) {
			return "";
		}

		$driver = strtolower(substr((string)$dsn, 0, $at));
		if ($driver === "postgresql") {
			return "postgres";
		}

		return $driver;
	}

	/** supported reports whether $driver is one dbadmin can talk to. */
	static function supported($driver) {
		return in_array($driver, driver_dao::DRIVERS);
	}

	/**
	 * bind returns $values checked as a list of scalars.
	 *
	 * A single array argument is bound by name rather than by position, so a
	 * statement that binds exactly one value and that value is an array
	 * silently becomes a named query and fails somewhere else. Refusing here
	 * makes the mistake say what it is.
	 */
	static function bind($values) {
		foreach ($values as $value) {
			if (is_array($value)) {
				throw new Exception("bound values must be scalars, got an array", 500);
			}
		}

		return array_copy($values);
	}

	/**
	 * run_all executes $sql against $db with $values bound, and returns all
	 * rows.
	 *
	 * The bound values are passed through the client's variadic argument
	 * list, which is what call_user_func_array is for; there is no argument
	 * unpacking in this runtime.
	 */
	static function run_all($db, $sql, $values) {
		$args = array_merge(array($sql), driver_dao::bind($values));
		return call_user_func_array($db->get_all, $args);
	}

	/** run_one executes $sql and returns its first row, or false. */
	static function run_one($db, $sql, $values) {
		$args = array_merge(array($sql), driver_dao::bind($values));
		return call_user_func_array($db->get, $args);
	}

	/** run executes $sql for its effect and returns true. */
	static function run($db, $sql, $values) {
		$args = array_merge(array($sql), driver_dao::bind($values));
		return call_user_func_array($db->query, $args);
	}

	/**
	 * verb returns the leading keyword of $sql, lowercased.
	 *
	 * A leading comment is skipped, because the query log tags statements
	 * that way and a tagged statement is still an insert.
	 */
	static function verb($sql) {
		$text = ltrim((string)$sql);
		while (str_starts_with($text, "/*")) {
			$end = strpos($text, "*/");
			if ($end === false) {
				return "";
			}

			$text = ltrim(substr($text, $end + 2));
		}

		if (preg_match("/^([A-Za-z]+)/", $text, $matches) != 1) {
			return "";
		}

		return strtolower($matches[1]);
	}
}
