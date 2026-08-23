name: destructuring into a property compiles to bytecode
description: >
  list() and foreach share one store path in the flatstack compiler, which
  accepts a property as a target. The stack order is the one an ordinary
  property assignment uses: value first, then the receiver.
---
<?php

class Stack
{
	public $name = "start";
	public $last = "";
	public $items = array("a", "b", "c");

	function pop()
	{
		list($this->name) = array_splice($this->items, -1);
		return $this->name;
	}

	function pair()
	{
		list($this->name, $this->last) = array("x", "y");
		return $this->name . $this->last;
	}

	function walk()
	{
		foreach ($this->items as $this->last) {
		}
		return $this->last;
	}
}

$stack = new Stack();
echo $stack->pop(), "\n";
echo implode(",", $stack->items), "\n";
echo $stack->pair(), "\n";
echo $stack->walk(), "\n";
echo $stack->name, "\n";
---
c
a,b
xy
b
x
