name: ksort and asort
description: >
  ksort, krsort, asort and arsort order an array in place with PHP's default
  comparison and keep every key attached to the value it belongs to, unlike
  sort() and rsort() which renumber from zero.
---
<?php
function dump($label, $array) {
    $parts = array();
    foreach ($array as $key => $value) {
        $parts[] = $key . "=>" . $value;
    }
    echo $label . ": " . implode(", ", $parts) . "\n";
}

// ksort and krsort order by key and carry the value along. An integer key
// meeting a non-numeric string key compares as a string, so 2 and 10 land
// before "id".
$mixed = array("id" => "one", 10 => "ten", 2 => "two", "name" => "four");
ksort($mixed);
dump("ksort", $mixed);
krsort($mixed);
dump("krsort", $mixed);

// asort and arsort order by value, and the key follows the value it belongs
// to rather than being handed out again from zero.
$widths = array("total" => 5, "id" => 12, "name" => 3, "created_at" => 19);
asort($widths);
dump("asort", $widths);
arsort($widths);
dump("arsort", $widths);

// Two numeric strings compare as numbers, so "9" sorts before "10" both as
// values and as keys.
$digits = array("a" => "100", "b" => "9", "c" => "1000", "d" => "10");
asort($digits);
dump("asort numeric", $digits);
arsort($digits);
dump("arsort numeric", $digits);

$byDigit = array("100" => "a", "9" => "b", "1000" => "c", "10" => "d");
ksort($byDigit);
dump("ksort numeric", $byDigit);

// An array that is already in order stays in that order, and every one of
// these returns true.
$done = array("a" => 1, "b" => 2, "c" => 3);
echo "returns: " . (ksort($done) ? "1" : "0") . (asort($done) ? "1" : "0") . "\n";
dump("already sorted", $done);
---
ksort: 2=>two, 10=>ten, id=>one, name=>four
krsort: name=>four, id=>one, 10=>ten, 2=>two
asort: name=>3, total=>5, id=>12, created_at=>19
arsort: created_at=>19, id=>12, total=>5, name=>3
asort numeric: b=>9, d=>10, a=>100, c=>1000
arsort numeric: c=>1000, a=>100, d=>10, b=>9
ksort numeric: 9=>b, 10=>d, 100=>a, 1000=>c
returns: 11
already sorted: a=>1, b=>2, c=>3
