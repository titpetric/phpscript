name: strtotime and date shims
description: >
  strtotime tries a fixed list of layouts from most to least specific and
  returns a Unix timestamp or false; date covers the numeric format
  characters. Inputs carrying an offset pin absolute epochs; naive inputs
  round-trip through date() so the fixture holds in any timezone.
---
<?php

// Offset-carrying layouts pin an absolute instant, most specific first.
var_dump(strtotime("2024-01-15T12:30:45.123456789+02:00"));
var_dump(strtotime("2024-01-15T12:30:45Z"));
var_dump(strtotime("Mon, 15 Jan 2024 12:30:45 +0200"));
var_dump(strtotime("Mon, 15 Jan 2024 12:30:45 GMT"));
var_dump(strtotime("@1705321845"));
var_dump(strtotime("@1705321845.5"));
var_dump(strtotime("now", 1705321845));

// Naive layouts read in the default timezone; the round trip through date()
// holds wherever the fixture runs.
echo date("Y-m-d H:i:s", strtotime("2024-01-15 12:30:45")), "\n";
echo date("Y-m-d H:i:s", strtotime("2024-01-15T12:30:45")), "\n";
echo date("Y-m-d H:i", strtotime("2024-01-15 12:30")), "\n";
echo date("Y-m-d", strtotime("  2024-01-15  ")), "\n";

// Numeric format characters, padded and bare, escapes and passthrough.
$ts = strtotime("2024-03-05 08:05:07");
echo date("y/n/j G:i:s", $ts), "\n";
echo date("Y-m-d\\TH:i:s", $ts), "\n";
echo date("\\Y=Y (Q)", $ts), "\n";
echo date("h g", strtotime("2024-01-15 22:30:00")), "\n";
echo date("h g", strtotime("2024-01-15 05:00:00")), "\n";
echo date("g", strtotime("2024-01-15 00:30:00")), " ", date("g", strtotime("2024-01-15 12:30:00")), "\n";
echo date("U", strtotime("2024-01-15T12:30:45Z")), "\n";

// No layout matches: false, as PHP returns it.
var_dump(strtotime("not a date"));
var_dump(strtotime(""));
?>
---
int(1705314645)
int(1705321845)
int(1705314645)
int(1705321845)
int(1705321845)
int(1705321845)
int(1705321845)
2024-01-15 12:30:45
2024-01-15 12:30:45
2024-01-15 12:30
2024-01-15
24/3/5 8:05:07
2024-03-05T08:05:07
Y=2024 (Q)
10 10
05 5
12 12
1705321845
bool(false)
bool(false)
