name: foreach over objects
description: >
  An object is a handle in PHP and here, so a `foreach` body that mutates the
  loop variable's properties edits the instance in the array whether or not the
  loop was written with `&`. Only the rebinding of the loop variable itself
  differs between the two spellings.
---
<?php

class Box {
	public $n = 1;
}

$boxes = array(new Box, new Box);
foreach ($boxes as $box) {
	$box->n = 7;
}
echo "value: " . $boxes[0]->n . "," . $boxes[1]->n . "\n";

$refBoxes = array(new Box, new Box);
foreach ($refBoxes as &$refBox) {
	$refBox->n = 9;
}
unset($refBox);
echo "reference: " . $refBoxes[0]->n . "," . $refBoxes[1]->n . "\n";

// Rebinding the loop variable is where the two differ: by value the array keeps
// its object, by reference the element becomes the replacement.
$swapped = array(new Box);
foreach ($swapped as $each) {
	$each = "replaced";
}
echo "value rebind: " . $swapped[0]->n . "\n";

$swappedRef = array(new Box);
foreach ($swappedRef as &$eachRef) {
	$eachRef = "replaced";
}
unset($eachRef);
echo "reference rebind: " . $swappedRef[0] . "\n";
---
value: 7,7
reference: 9,9
value rebind: 1
reference rebind: replaced
