name: is_object
description: >
  is_object answers for a value a Go binding returned, not only for an
  interpreted object. Exception is the case both runtimes can be held to: it is
  a Go type here and a class in PHP, and both call it an object. The expected
  output is what php prints for the same source.
---
<?php

class PhpThing {
    public $x = 1;
}

function show($label, $v) {
    echo $label . "=" . (is_object($v) ? "object" : "not_object") . "\n";
}

show("php_object", new PhpThing());
show("exception", new Exception("boom"));
show("array", array(1, 2, 3));
show("empty_array", array());
show("string", "x");
show("int", 1);
show("float", 1.5);
show("bool", true);
show("null", null);
?>
---
php_object=object
exception=object
array=not_object
empty_array=not_object
string=not_object
int=not_object
float=not_object
bool=not_object
null=not_object
