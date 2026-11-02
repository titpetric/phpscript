<?php

class Row {
	public $id;
	public $name;
}

function fill($object, $fields, $row) {
	foreach ($fields as $name) {
		if (isset($row[$name])) {
			$object->$name = $row[$name];
		}
	}
	return $object;
}

$field = "name";
echo $r->$field;
$r->{$field . "suffix"} = "value";
unset($r->$field);
