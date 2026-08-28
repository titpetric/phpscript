name: new takes a class name held in a variable, an index or a property
description: >
  The name a `new` resolves is a class name reference, not a bare variable: it
  can sit in an array element or on a property, and the parenthesis after it
  opens the constructor arguments rather than a call on the result. Reading
  only the variable took `new $map["png"]($file)` as
  `(new $map)["png"]($file)`, constructing whatever $map named and then
  subscripting it.

  The calling side does not mirror that, and the difference is deliberate.
  PHP 7's uniform variable syntax made `$obj->$calls["read"]()` mean
  `($obj->$calls)["read"]()`, a dynamic property that is then subscripted, so
  the index is left to the outer expression. A method named by anything other
  than a plain variable is spelled with braces, `$obj->{$calls["read"]}()`,
  which is what PHP has taken since 7.0.
---
<?php

class Png
{
	public $tag;

	public function __construct($tag = "")
	{
		$this->tag = $tag;
	}

	public function get()
	{
		return "png:" . $this->tag;
	}

	public function size()
	{
		return strlen($this->tag);
	}
}

class Registry
{
	public $class = "Png";
	public $calls = array("read" => "get", "count" => "size");
}

// The name in a plain variable.
$name = "Png";
$plain = new $name("a");
echo $plain->get(), "\n";

// The name in an array element, with constructor arguments after it.
$types = array("image/png" => "Png");
$indexed = new $types["image/png"]("b");
echo $indexed->get(), "\n";

// The name on a property.
$registry = new Registry();
$viaProp = new $registry->class("c");
echo $viaProp->get(), "\n";

// No arguments at all: the reference ends at the semicolon.
$bare = new $name;
echo $bare->get(), "\n";

// A nested index resolves the same way.
$nested = array("mime" => array("png" => "Png"));
echo (new $nested["mime"]["png"]("d"))->get(), "\n";

// A method named by a plain variable.
$one = "get";
echo $indexed->$one(), "\n";

// A method named by anything else takes braces.
$calls = array("read" => "get", "count" => "size");
echo $indexed->{$calls["read"]}(), "\n";
echo $indexed->{$calls["count"]}(), "\n";
echo $indexed->{$registry->calls["read"]}(), "\n";

// The braces take an expression, not only a lookup.
echo $indexed->{"g" . "et"}(), "\n";
---
png:a
png:b
png:c
png:
png:d
png:b
png:b
1
png:b
png:b
