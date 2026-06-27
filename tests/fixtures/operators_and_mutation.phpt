name: operators and mutation syntax
description: >
  Covers PHP-style value mutation syntax: prefix/postfix increment/decrement,
  compound assignment, array append, indexed assignment, and using postfix
  increment as an array key/index value.
---
<?php

$i = 0;
$items = array();
$items[] = "zero";
$items[] = "one";

$ids = array();
$ids[] = array("id" => $i++);
$ids[] = array("id" => $i++);
$ids[] = array("id" => $i++);

echo $ids[0]["id"] . "," . $ids[1]["id"] . "," . $ids[2]["id"] . ";";
echo $i . ";";

$i--;
echo $i . ";";
echo $i-- . ";";
echo $i . ";";
echo ++$i . ";";
echo $i . ";";
echo --$i . ";";
echo $i . ";";

$n = 5;
$n += 3;
$n -= 2;
echo $n . ";";

$s = "foo";
$s .= "bar";
echo $s . ";";

$arr = array();
$arr[] = "foo";
$arr[] = "bar";
$arr[1] = "baz";
echo $arr[0] . "," . $arr[1] . ";";

$nums = array(10, 20);
echo $nums[0]++ . "," . $nums[0] . ";";
echo ++$nums[1] . "," . $nums[1] . ";";

class Counter {
  var $n = 5;
}
$counter = new Counter;
echo $counter->n++ . "," . $counter->n . ";";
echo ++$counter->n . "," . $counter->n . ";";
echo $counter->n-- . "," . $counter->n . ";";
echo --$counter->n . "," . $counter->n . ";";
---
0,1,2;3;2;2;1;2;2;1;1;6;foobar;foo,baz;10,11;21,21;5,6;7,7;7,6;5,5;
