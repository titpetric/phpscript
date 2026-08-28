name: object properties read back in the order PHP gives them
description: >
  A property added by assignment reads back where it was added, not sorted by
  name. The order has two parts, and PHP keeps them apart: a declared property
  holds the slot its declaration gave it, so unsetting one and assigning it
  again puts it back in the same place, while a dynamic property has no slot to
  return to and moves to the end. Assigning over a property that is still set
  never moves it. Every reader agrees, because they read one order rather than
  each deriving its own: json_encode, print_r, var_dump, get_object_vars, the
  (array) cast and foreach.
---
<?php

class Row
{
	public $id = 1;
	public $name = "first";
}

$row = new Row();
$row->zeta = "z";
$row->alpha = "a";

// Declared properties come first, in declaration order; the dynamic ones
// follow in the order they were assigned, not sorted.
echo json_encode($row), "\n";
echo implode(",", array_keys(get_object_vars($row))), "\n";
echo implode(",", array_keys((array) $row)), "\n";
print_r($row);

// A declared property that is unset and assigned again returns to its
// declared slot.
unset($row->id);
$row->id = 9;
echo json_encode($row), "\n";

// A dynamic one moves to the end instead: it has no slot to return to.
unset($row->zeta);
$row->zeta = "Z";
echo json_encode($row), "\n";

// Assigning to a property that is still set leaves it where it is.
$row->alpha = "A";
echo json_encode($row), "\n";

var_dump($row);

$bag = new stdClass();
$bag->c = 3;
$bag->a = 1;
$bag->b = 2;
echo json_encode($bag), "\n";
foreach ($bag as $key => $value) {
	echo $key;
}
echo "\n";
unset($bag->c);
$bag->c = 30;
echo json_encode($bag), "\n";
---
{"id":1,"name":"first","zeta":"z","alpha":"a"}
id,name,zeta,alpha
id,name,zeta,alpha
Row Object
(
    [id] => 1
    [name] => first
    [zeta] => z
    [alpha] => a
)
{"id":9,"name":"first","zeta":"z","alpha":"a"}
{"id":9,"name":"first","alpha":"a","zeta":"Z"}
{"id":9,"name":"first","alpha":"A","zeta":"Z"}
object(Row)#1 (4) {
  ["id"]=>
  int(9)
  ["name"]=>
  string(5) "first"
  ["alpha"]=>
  string(1) "A"
  ["zeta"]=>
  string(1) "Z"
}
{"c":3,"a":1,"b":2}
cab
{"a":1,"b":2,"c":30}
