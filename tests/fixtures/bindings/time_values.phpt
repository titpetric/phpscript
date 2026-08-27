name: Go time values render and destructure
description: >
  A Time, a Duration and a Location echo through their Go stringer rather
  than as an empty string, a named integer counts as an integer in
  arithmetic, and a Go method with several results arrives as a PHP list.
  php has no counterpart: it refuses to convert a DateTime to a string at
  all, and returns none of these as tuples.
runner:
  php: false
---
<?php

set_timezone("UTC");

$t = DateTime::parse("2006-01-02 15:04:05", "2026-08-26 14:48:00");

echo $t, "\n";
echo "{$t}\n";
echo new Time\Duration("1h30m"), "\n";
echo new Time\Location("Europe/Ljubljana"), "\n";

echo $t->month(), "\n";
echo $t->weekday(), "\n";
echo $t->month() + 1, "\n";

list($year, $week) = $t->iso_week();
echo $year, " ", $week, "\n";

list($hour, $min, $sec) = $t->clock();
echo $hour, ":", $min, ":", $sec, "\n";

list($name, $offset) = $t->zone();
echo $name, " ", $offset, "\n";
---
2026-08-26 14:48:00
2026-08-26 14:48:00
1h30m0s
Europe/Ljubljana
August
Wednesday
9
2026 35
14:48:0
UTC 0
