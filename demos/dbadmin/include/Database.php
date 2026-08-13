<?php

class Database {
	protected $handle;

	public function connect($name) {
		$this->handle = new PS\Database($name);
		$this->handle->enableTracing();
	}

	public function query($sql, $values = false) {
		$statement = $this->handle->prepare($sql);
		if (is_array($values)) {
			foreach ($values as $key => $value) {
				$statement->bindValue(":" . $key, $value);
			}
		} else {
			$values = array_slice(func_get_args(), 1);
			$position = 1;
			foreach ($values as $value) {
				$statement->bindValue($position, $value);
				$position++;
			}
		}

		$statement->execute();
		return $statement;
	}

	public function get($sql) {
		$statement = call_user_func_array($this->query, func_get_args());
		$row = $statement->fetch();

		$statement->close();
		return $row;
	}

	public function get_all($sql) {
		$statement = call_user_func_array($this->query, func_get_args());
		$rows = array();
		$row = $statement->fetch();
		while ($row) {
			$rows[] = $row;
			$row = $statement->fetch();
		}

		$statement->close();
		return $rows;
	}

	public function close() {
		$this->handle->close();
	}
}
