name: sort
flatstack: true
description: >
  sort and rsort order a list in place, numerically when the values are
  numbers and by string otherwise, reindexing the keys from zero.
---
<?php
$numbers = array(10, 2, 33, 4);
sort($numbers);
echo implode(",", $numbers) . "\n";

rsort($numbers);
echo implode(",", $numbers) . "\n";

$words = array("total", "id", "name", "created_at");
sort($words);
echo implode(",", $words) . "\n";

$keyed = array("b" => "two", "a" => "one");
sort($keyed);
echo $keyed[0] . "," . $keyed[1] . "\n";
---
2,4,10,33
33,10,4,2
created_at,id,name,total
one,two
