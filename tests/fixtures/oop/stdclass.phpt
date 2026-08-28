name: stdClass is a property bag with no members of its own
description: >
  `new stdClass` gives an object that declares nothing: every property is added
  by assignment, reads back in the order it was added, and answers isset,
  foreach, get_object_vars and instanceof the way any other object does. An
  object is a handle in PHP as well as here, so a function that adds a property
  to its argument, and a second name for the same object, are both visible
  through the original. The dump functions need no special case: print_r,
  var_dump and json_encode already read *model.Object, and var_export prints
  the `(object) array(...)` form PHP uses for a class with no __set_state.
---
<?php

$o = new stdClass();
$o->name = "phpscript";
$o->count = 2;

echo get_class($o), "\n";
echo $o->name, ":", $o->count, "\n";
echo isset($o->name) ? "set" : "unset", "\n";
echo isset($o->missing) ? "set" : "unset", "\n";
echo $o instanceof stdClass ? "stdClass" : "not stdClass", "\n";
echo is_object($o) ? "object" : "not object", "\n";
echo gettype($o), "\n";

$bare = new stdClass;
$bare->only = 1;
echo count(get_object_vars($bare)), "\n";

foreach ($o as $key => $value) {
	echo "  $key=$value\n";
}

$o->child = new stdClass();
$o->child->deep = "nested";
$o->list = array(1, 2, 3);
echo $o->child->deep, "\n";
echo count($o->list), "\n";

function annotate($obj)
{
	$obj->added = "by function";
}

annotate($o);
echo $o->added, "\n";

$alias = $o;
$alias->shared = "yes";
echo $o->shared, "\n";

unset($o->child, $o->list, $o->added, $o->shared);
echo json_encode($o), "\n";
print_r($o);
var_dump($o);
var_export($o);
echo "\n";
echo json_encode(new stdClass()), "\n";
---
stdClass
phpscript:2
set
unset
stdClass
object
object
1
  name=phpscript
  count=2
nested
3
by function
yes
{"name":"phpscript","count":2}
stdClass Object
(
    [name] => phpscript
    [count] => 2
)
object(stdClass)#1 (2) {
  ["name"]=>
  string(9) "phpscript"
  ["count"]=>
  int(2)
}
(object) array(
   'name' => 'phpscript',
   'count' => 2,
)
{}
