name: string interpolation, simple syntax
description: >
  A double-quoted literal evaluates the variables written into it: a bare
  $name, a $name[key] subscript whose bare word is a string key rather than a
  constant, and one level of $name->prop. A single-quoted literal never
  interpolates, an escaped \$ stays a dollar, and a dollar that starts no name
  is literal text.
---
<?php

class Row {
	var $id = 7;
	var $child = null;
}

$name = "Ada";
$i = 0;
$n = 42;
$f = 1.5;
$yes = true;
$nothing = null;
$row = array("id" => "R1", 0 => "zero");
$obj = new Row();
$obj->child = new Row();
$obj->child->id = "inner";

echo "simple=$name\n";
echo "adjacent=$name$name\n";
echo "word=$row[id]\n";
echo "digit=$row[0]\n";
echo "var=$row[$i]\n";
echo "prop=$obj->id\n";
echo "onelevel=$obj->id->x\n";
echo "int=$n float=$f\n";
echo "bool=$yes| null=$nothing|\n";
echo "trailing=$name's\n";
echo "after=$row[id]x\n";
echo "propafter=$obj->id!\n";
echo "escaped=\$name\n";
echo "lone=$ name\n";
echo 'single=$name', "\n";
echo "empty=", "$name" === $name ? "same" : "different", "\n";
---
simple=Ada
adjacent=AdaAda
word=R1
digit=zero
var=zero
prop=7
onelevel=7->x
int=42 float=1.5
bool=1| null=|
trailing=Ada's
after=R1x
propafter=7!
escaped=$name
lone=$ name
single=$name
empty=same
