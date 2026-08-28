name: (object) and (array) convert between arrays and stdClass
description: >
  `(object)` builds a stdClass whose properties are the array's entries, with an
  integer key becoming a property named for its digits; a scalar lands under
  `scalar`, null gives an empty object, and an object is returned as it is
  rather than copied. `(array)` reads the properties back out in the order they
  were set, running each name through the array-key rules so a digit-named
  property is an integer key again and a cast round trip of a list is a list.
  Neither direction recurses: an array inside the value stays an array. The
  properties of a declared class cast the same way as the dynamic ones of a
  stdClass, and the cast reaches the value through both engines because both
  route through the same helper.
---
<?php

$assoc = array("host" => "localhost", "port" => 8080);
$o = (object) $assoc;
echo get_class($o), "\n";
echo $o->host, ":", $o->port, "\n";
echo json_encode($o), "\n";

// An integer key becomes a property whose name is the digits.
$list = (object) array("first", "second");
echo json_encode($list), "\n";
echo implode(",", array_keys(get_object_vars($list))), "\n";

// A scalar lands under `scalar`; null gives an object with nothing in it.
$s = (object) "text";
echo get_class($s), " ", $s->scalar, "\n";
echo ((object) 5)->scalar, "\n";
echo ((object) 1.5)->scalar, "\n";
echo ((object) true)->scalar ? "true" : "false", "\n";
$empty = (object) null;
echo get_class($empty), " ", count(get_object_vars($empty)), "\n";

// Casting an object is the identity, not a copy.
$again = (object) $o;
echo $again === $o ? "same" : "different", "\n";

// (array) reads the properties back out, keys in the order they were set.
$back = (array) $o;
echo is_array($back) ? "array" : "not array", "\n";
echo implode(",", array_keys($back)), "\n";
echo implode(",", $back), "\n";
echo gettype($back), "\n";

// A digit-named property comes back as an integer key, so a cast round trip
// of a list is a list again.
$roundtrip = (array) $list;
var_dump(array_keys($roundtrip));
echo implode(",", $roundtrip), "\n";

// A declared class casts the same way.
class Point
{
	public $x = 1;
	public $y = 2;
}

$point = new Point();
$point->label = "origin";
echo implode(",", array_keys((array) $point)), "\n";

// Nesting is not converted: an array inside stays an array.
$deep = (object) array("rows" => array("n" => 1));
echo is_array($deep->rows) ? "array" : "not array", "\n";
echo $deep->rows["n"], "\n";

// (array) of a scalar and of null, unchanged by any of this.
var_dump((array) "text");
var_dump((array) null);
---
stdClass
localhost:8080
{"host":"localhost","port":8080}
{"0":"first","1":"second"}
0,1
stdClass text
5
1.5
true
stdClass 0
same
array
host,port
localhost,8080
array
array(2) {
  [0]=>
  int(0)
  [1]=>
  int(1)
}
first,second
x,y,label
array
1
array(1) {
  [0]=>
  string(4) "text"
}
array(0) {
}
