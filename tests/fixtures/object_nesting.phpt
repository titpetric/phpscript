name: object and value nesting
runner:
  php: false
description: >
  Test chained property, method, and array access in reads and foreach sources.
  The classes declare PHP 4 style constructors, which minitpl relies on and
  PHP 8 removed, so the expected output is phpscript's contract.
---
<?php
class Leaf {
	var $value;
	var $items;

	function Leaf($value, $items) {
		$this->value = $value;
		$this->items = $items;
	}

	function label($prefix) {
		return $prefix . $this->value;
	}
}

class Branch {
	var $someValue;
}

class Root {
	var $someObject;
}

$a = new Root;
$a->someObject = new Branch;
$a->someObject->someValue = new Leaf("nested", array("x", "y"));

echo $a->someObject->someValue->value;
echo "|" . $a->someObject->someValue->label("method-");

$wrapped = array("root" => $a);
echo "|" . $wrapped['root']->someObject->someValue->items[1];

$state = array();
foreach ($wrapped['root']->someObject->someValue->items as $state['key'] => $state['value']) {
	echo "|" . $state['key'] . "=" . $state['value'];
}
?>
---
nested|method-nested|y|0=x|1=y
