name: sort
flatstack: true
description: >
  sort and rsort order a list in place with PHP's comparison operator,
  reindexing the keys from zero: numbers and numeric strings compare as
  numbers, null and bool compare as bools, and everything else compares as
  strings.
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

// Two numeric strings compare as numbers, so "9" comes before "10".
$digits = explode(",", "100,1,1000,10");
sort($digits);
echo implode(",", $digits) . "\n";

rsort($digits);
echo implode(",", $digits) . "\n";

// A number meeting a string that is not numeric compares as a string, which
// puts "abc" after 9 but "1e2" (numeric) before it.
$mixed = array("abc", "10", "2", "1e2", 9);
sort($mixed);
echo json_encode($mixed) . "\n";

// null and bool compare on truthiness whatever the other side is.
$flags = array(true, null, false);
sort($flags);
echo json_encode($flags) . "\n";

// null against a string is the exception: the null becomes "".
$maybe = array("a", null, "");
sort($maybe);
echo json_encode($maybe) . "\n";

// An array is greater than any scalar, and the shorter array is smaller.
$rows = array(array(1, 2), array(3), 5);
sort($rows);
echo json_encode($rows) . "\n";
---
2,4,10,33
33,10,4,2
created_at,id,name,total
one,two
1,10,100,1000
1000,100,10,1
["2",9,"10","1e2","abc"]
[null,false,true]
[null,"","a"]
[5,[3],[1,2]]
