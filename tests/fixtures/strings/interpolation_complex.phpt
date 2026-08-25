name: string interpolation, complex syntax
description: >
  The braces of {$expr} re-enter PHP, so the expression inside is an ordinary
  one: a quoted array key, a nested subscript, a chain of properties, a method
  call, and arithmetic. The scan closes on the brace that matches, not on one
  written inside a key, and a brace that starts no expression is literal text.
---
<?php

class Node {
	var $name = "root";
	var $child = null;

	function label() {
		return "[" . $this->name . "]";
	}
}

$rows = array(
	"a" => array("b" => "deep"),
	"}" => "brace-key",
	0 => "first",
);
$node = new Node();
$node->child = new Node();
$node->child->name = "leaf";
$i = 0;
$n = 4;

echo "key={$rows['a']['b']}\n";
echo "int={$rows[0]}\n";
echo "expr={$rows[$i]}\n";
echo "bracekey={$rows['}']}\n";
echo "prop={$node->child->name}\n";
echo "method={$node->label()}\n";
echo "nested={$node->child->label()}\n";
echo "arith={$rows['a']['b']}-{$n}\n";
echo "joined={$node->name}{$node->child->name}\n";
echo "text={ $n }\n";
echo "notexpr={x}\n";
echo "adjacent=a{$n}b\n";
---
key=deep
int=first
expr=first
bracekey=brace-key
prop=leaf
method=[root]
nested=[leaf]
arith=deep-4
joined=rootleaf
text={ 4 }
notexpr={x}
adjacent=a4b
