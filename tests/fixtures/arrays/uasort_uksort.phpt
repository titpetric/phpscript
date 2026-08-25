name: uasort and uksort
description: >
  uasort and uksort order an array in place with a script comparator, uasort
  comparing the values and uksort the keys, and both keep the key-to-value
  association that usort() throws away.
---
<?php
function dump($label, $array) {
    $parts = array();
    foreach ($array as $key => $value) {
        $parts[] = $key . "=>" . $value;
    }
    echo $label . ": " . implode(", ", $parts) . "\n";
}

function byLength($a, $b) {
    return strlen($a) - strlen($b);
}

// uasort compares values and keeps the key attached to the value it moved.
$widths = array("total" => 5, "id" => 12, "name" => 3, "created_at" => 19);
uasort($widths, function ($a, $b) { return $a - $b; });
dump("uasort asc", $widths);
uasort($widths, function ($a, $b) { return $b - $a; });
dump("uasort desc", $widths);

// The comparator sees the values, so it can order by something the default
// comparison would not, here the last character of the string.
$codes = array("a" => "x9", "b" => "y1", "c" => "z5");
uasort($codes, function ($x, $y) {
    if (substr($x, 1) == substr($y, 1)) {
        return 0;
    }
    return (substr($x, 1) < substr($y, 1)) ? -1 : 1;
});
dump("uasort by suffix", $codes);

// uksort compares keys. Ties keep their original order, so "id" stays ahead
// of "ts".
$cols = array("created_at" => 1, "id" => 2, "name" => 3, "ts" => 4);
uksort($cols, "byLength");
dump("uksort by length", $cols);

$rev = function ($a, $b) {
    if ($a == $b) {
        return 0;
    }
    return ($a < $b) ? 1 : -1;
};
uksort($cols, $rev);
dump("uksort reversed", $cols);

// An array the comparator already agrees with is left alone, and both
// functions return true.
$done = array("a" => 1, "b" => 2, "c" => 3);
$asc = function ($x, $y) { return $x - $y; };
echo "returns: " . (uasort($done, $asc) ? "1" : "0") . (uksort($done, "byLength") ? "1" : "0") . "\n";
dump("already sorted", $done);
---
uasort asc: name=>3, total=>5, id=>12, created_at=>19
uasort desc: created_at=>19, id=>12, total=>5, name=>3
uasort by suffix: b=>y1, c=>z5, a=>x9
uksort by length: id=>2, ts=>4, name=>3, created_at=>1
uksort reversed: ts=>4, name=>3, id=>2, created_at=>1
returns: 11
already sorted: a=>1, b=>2, c=>3
