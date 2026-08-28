name: an anonymous class returned from an included file
description: >
  `new class implements TestInterface { ... }` declares a class with no name and
  builds it in one expression. The declaration lives in a support file whose
  only export is the value it returns, and the interface it names is declared by
  the file that includes it, so nothing but the returned object connects the
  two. Once built it is an ordinary object: instanceof answers the interface,
  methods see $this, a property can be added to it and it is a handle. A second
  include builds a second instance. get_class() is not printed, because PHP
  names an anonymous class after the file and line that declared it. The
  bytecode engine does not compile an anonymous class, so this fixture runs on
  the interpreter through the documented whole-program fallback.
---
<?php

interface TestInterface
{
	public function name();
	public function greet($who);
}

$obj = require 'testdata/factory.php';

echo is_object($obj) ? "object" : "not object", "\n";
echo $obj instanceof TestInterface ? "TestInterface" : "not TestInterface", "\n";
echo $obj->label, "\n";
echo $obj->name(), "\n";
echo $obj->greet("world"), "\n";
echo $obj->greet("again"), "\n";
echo $obj->calls(), "\n";

// It is an ordinary object once it exists: a property can be added to it and
// it is a handle, so the second name sees the same one.
$obj->extra = "added";
$alias = $obj;
$alias->label = "renamed";
echo $obj->label, ":", $obj->extra, "\n";
echo method_exists($obj, "greet") ? "has greet" : "no greet", "\n";
echo property_exists($obj, "label") ? "has label" : "no label", "\n";

// Each evaluation of the include builds a new instance.
$second = require 'testdata/factory.php';
echo $second === $obj ? "same" : "different", "\n";
echo $second->name(), "\n";
---
object
TestInterface
anonymous
anonymous
hello world
hello again
2
renamed:added
has greet
has label
different
anonymous
