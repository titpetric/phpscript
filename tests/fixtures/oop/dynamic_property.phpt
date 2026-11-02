name: dynamic property names
description: >
  A property named by a variable or a braced expression can be read, written,
  appended, incremented and unset, and isset reports the property it names.
---
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

$r = fill(new Row(), array("id", "name"), array("id" => 7, "name" => "note"));
echo $r->id, " ", $r->name, "\n";

$field = "name";
echo $r->{$field}, "\n";
$r->{$field} = "changed";
echo $r->name, "\n";
$r->$field .= "!";
echo $r->name, "\n";

$field = "counter";
$r->$field = 0;
$r->$field++;
echo $r->counter, "\n";

unset($r->$field);
var_dump(isset($r->counter));

$key = "na";
$r->{$key . "me"} = "built";
echo $r->name, "\n";
---
7 note
note
changed
changed!
1
bool(false)
built
