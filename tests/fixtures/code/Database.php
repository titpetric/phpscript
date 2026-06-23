<?php

class Database
{
	protected $handle;

	public function connect($connection_name)
	{
		$this->handle = new DatabaseDriver($connection_name);
	}

	public function close() {
		$this->handle->close();
	}

	public function insert($table, $values)
	{
		$keys = array_keys($values);
		$query = "insert into " . $table . " (" . implode(", ", $keys) . ") values (" . implode(", ", array_map(function($s) { return ":".$s; }, $keys)) . ")";
		return $this->query($query, $values);
	}

	public function replace($table, $values, $extra_sql = '')
	{
		$keys = array_keys($values);
		$query = "replace into " . $table . " (" . implode(", ", $keys) . ") values (" . implode(", ", array_map(function($s) { return ":".$s; }, $keys)) . ") " . $extra_sql;
		return $this->query($query, $values);
	}

	public function update($table, $values, $keys)
	{
		if (!is_array($keys)) {
			$keys = array($keys);
		}
		$query = "update " . $table . " set ";
		$index = 0;
		foreach ($values as $k => $v) {
			if (!in_array($k, $keys)) {
				if ($index > 0) {
					$query .= ', ';
				}
				$query .= "$k = :$k";
				$index = $index + 1;
			}
		}
		$query .= " where ";
		$index = 0;
		foreach ($keys as $k) {
			$v = $values[$k];
			if ($index > 0) {
				$query .= ' and ';
			}
			$query .= "$k = :$k";
			$index = $index + 1;
		}
		return $this->query($query, $values);
	}

	public function query($query, $values = false)
	{
		$stmt = $this->handle->prepare($query);
		if (is_array($values)) {
			foreach ($values as $k => $v) {
				$stmt->bindValue(":" . $k, $v);
			}
		} else {
			$values = array_slice(func_get_args(), 1);
			$idx = 1;
			foreach ($values as $v) {
				$stmt->bindValue($idx, $v);
				$idx = $idx + 1;
			}
		}
		$stmt->execute();
		return $stmt;
	}

	public function get()
	{
		$stmt = call_user_func_array($this->query, func_get_args());
		$row = $this->fetch($stmt);
		$stmt->closeCursor();
		return $row;
	}

	public function get_all()
	{
		$stmt = call_user_func_array($this->query, func_get_args());
		$rows = array();
		while ($row = $this->fetch($stmt)) {
			$rows[] = $row;
		}
		return $rows;
	}

	public function fetch($stmt)
	{
		$row = $stmt->fetch();
		if (empty($row)) {
			$stmt->closeCursor();
		}
		return $row;
	}

	public function insert_id()
	{
		return $this->handle->lastInsertId();
	}

	/* Implementation of ITransactional interface */

	protected $in_transaction = 0;

	public function begin()
	{
		if ($this->in_transaction == 0) {
			$r = $this->query("BEGIN");
		} else {
			$r = $this->query("SAVEPOINT sp_".$this->in_transaction);
		}
		$this->in_transaction = $this->in_transaction + 1;
		if (!$r) {
			throw new Exception("Database error: can't start transaction");
		}
		return $r;
	}

	public function start()
	{
		return $this->begin();
	}

	public function rollback()
	{
		if ($this->in_transaction > 0) {
			$this->in_transaction = $this->in_transaction - 1;
		}
		if ($this->in_transaction == 0) {
			$r = $this->query("ROLLBACK");
		} else {
			$r = $this->query("ROLLBACK TO SAVEPOINT sp_".$this->in_transaction);
		}
		return $r;
	}

	public function commit()
	{
		if ($this->in_transaction > 0) {
			$this->in_transaction = $this->in_transaction - 1;
		}
		if ($this->in_transaction == 0) {
			$r = $this->query("COMMIT");
		} else {
			$r = $this->query("RELEASE SAVEPOINT sp_".$this->in_transaction);
		}
		return $r;
	}
}
