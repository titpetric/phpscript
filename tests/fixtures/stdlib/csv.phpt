name: fputcsv quotes fields that need it and only those
description: >
  A quote is doubled, a comma or newline forces enclosure, plain fields stay
  bare, and fgetcsv reads the records back. The php runner is opted out
  because this binding is RFC 4180 by construction, where PHP's default
  $escape produces non-standard CSV (deprecated as of PHP 8.4).
runner:
  php: false
---
<?php

$h = fopen("php://output", "w");
fputcsv($h, array("plain", "with,comma", "with\"quote", "multi\nline"));
fputcsv($h, array("1.5", "x"));
fclose($h);

$f = "fixture-csv.txt";
$w = fopen($f, "w");
var_dump(fputcsv($w, array("a", "b,c")));
fputcsv($w, array("semi", "colon"), ";");
fclose($w);

$r = fopen($f, "r");
while (($row = fgetcsv($r)) !== false) {
    echo implode("|", $row), "\n";
}
fclose($r);
unlink($f);
---
plain,"with,comma","with""quote","multi
line"
1.5,x
int(8)
a|b,c
semi;colon
