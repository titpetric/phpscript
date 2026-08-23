<?php

/**
 * row_dao writes single rows to a target table.
 *
 * Statements are assembled here rather than through the client's insert() and
 * update() helpers. Those build their own SQL without knowing the driver's
 * placeholder style, and insert_id() reports nothing useful on postgres, so a
 * write that has to work on all three is written out.
 *
 * delete() takes the destructive decision as an argument and checks it again.
 * The controller checks first; a controller that forgot to is then a refusal
 * rather than a deletion.
 */
class row_dao {
	public $tables;
	public $audit;

	function __construct($tables, $audit) {
		$this->tables = $tables;
		$this->audit = $audit;
	}

	/**
	 * insert adds a row built from $input, which maps column names to the
	 * posted strings.
	 *
	 * A column absent from $input is left to its default. A column present
	 * and marked null in $nulls is written as NULL, which is a different
	 * thing from the empty string and is the distinction a form cannot make
	 * on its own.
	 */
	function insert($db, $driver, $schema, $table, $input, $nulls, $ctx) {
		$columns = $this->tables->columns($db, $driver, $schema, $table);
		$writable = row_dao::collect($columns, $input, $nulls);

		if (count($writable["names"]) == 0) {
			throw new Exception("No columns were given a value.", 400);
		}

		$quoted = array();
		foreach ($writable["names"] as $name) {
			$quoted[] = driver_dao::quote_ident($driver, $name);
		}

		$sql = "INSERT INTO " . driver_dao::qualify($driver, $schema, $table) . " (" . implode(", ", $quoted) . ")" . " VALUES (" . driver_dao::placeholders($driver, count($writable["values"]), 1) . ")";

		driver_dao::run($db, $sql, $writable["values"]);

		$this->audit->log($ctx, "insert", $table, "", "inserted a row into " . $table, array("schema" => $schema, "columns" => $writable["names"]));
	}

	/** update rewrites the row addressed by $key. */
	function update($db, $driver, $schema, $table, $key, $input, $nulls, $ctx) {
		$columns = $this->tables->columns($db, $driver, $schema, $table);
		$identity = $this->tables->identity($db, $driver, $schema, $table);
		if ($identity["kind"] === "none") {
			throw new Exception("This table has no primary key, so a row cannot be updated.", 409);
		}

		$writable = row_dao::collect($columns, $input, $nulls);
		if (count($writable["names"]) == 0) {
			throw new Exception("No columns were given a value.", 400);
		}

		$assignments = array();
		$position = 1;
		foreach ($writable["names"] as $name) {
			$assignments[] = driver_dao::quote_ident($driver, $name) . " = " . driver_dao::placeholder($driver, $position);
			$position += 1;
		}

		$where = browse_dao::key_where($driver, $identity, $key, $position);
		$sql = "UPDATE " . driver_dao::qualify($driver, $schema, $table) . " SET " . implode(", ", $assignments) . " WHERE " . $where["sql"];

		driver_dao::run($db, $sql, array_merge(array_copy($writable["values"]), $where["values"]));

		$this->audit->log($ctx, "update", $table, $key, "updated a row in " . $table, array("schema" => $schema, "columns" => $writable["names"]));
	}

	/**
	 * delete removes the row addressed by $key.
	 *
	 * $can_destroy is the decision acl_dao made for this session. It is
	 * re-checked here so that the guard is on the statement rather than only
	 * on the page that leads to it.
	 */
	function delete($db, $driver, $schema, $table, $key, $can_destroy, $ctx) {
		if (!$can_destroy) {
			$this->audit->log($ctx, "denied", $table, $key, "refused to delete a row from " . $table, array("schema" => $schema));
			throw new Exception("Destructive actions are not enabled for this session.", 403);
		}

		$identity = $this->tables->identity($db, $driver, $schema, $table);
		if ($identity["kind"] === "none") {
			throw new Exception("This table has no primary key, so a row cannot be deleted.", 409);
		}

		$where = browse_dao::key_where($driver, $identity, $key, 1);
		$sql = "DELETE FROM " . driver_dao::qualify($driver, $schema, $table) . " WHERE " . $where["sql"];

		driver_dao::run($db, $sql, $where["values"]);

		$this->audit->log($ctx, "delete", $table, $key, "deleted a row from " . $table, array("schema" => $schema));
	}

	/**
	 * collect reads the posted form into a column list and a value list.
	 *
	 * Only declared columns are read, so an extra field in the request body
	 * cannot name a column that was not offered.
	 */
	static function collect($columns, $input, $nulls) {
		$names = array();
		$values = array();

		foreach ($columns as $column) {
			$name = $column["name"];
			if (!array_key_exists($name, $input)) {
				continue;
			}

			$names[] = $name;
			if (array_key_exists($name, $nulls)) {
				$values[] = null;
			} else {
				$values[] = (string)$input[$name];
			}
		}

		return array("names" => $names, "values" => $values);
	}

	/**
	 * blank returns an empty row shaped like $columns, for the insert form.
	 *
	 * A column with a default is left empty and omitted on submit, so the
	 * default applies; the form says so beside the field.
	 */
	static function blank($columns) {
		$row = array();
		foreach ($columns as $column) {
			$row[$column["name"]] = "";
		}

		return $row;
	}
}
