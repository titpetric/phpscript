name: Go time bindings
description: >
  Time exposes Go time.Time methods, duration arguments accept Go duration
  strings, and set_timezone binds parsing and construction to this runtime.
runner:
  php: false
---
<?php

set_timezone("America/New_York");

$start = DateTime::parse("2006-01-02 15:04:05", "2026-08-26 14:48:00");
echo $start->format("2006-01-02 15:04 MST"), "\n";

$later = $start->add("30m");
echo $later->format("15:04"), "\n";

$duration = new Time\Duration("1h30m");
echo $duration->minutes(), "\n";
echo $start->add($duration)->format("15:04"), "\n";

$utc = new Time\Location("UTC");
set_timezone($utc);
$parsed = DateTime::parse("2006-01-02 15:04:05", "2026-08-26 14:48:00");
echo $parsed->format("15:04 MST"), "\n";
echo Time\Duration::parse("45s")->seconds(), "\n";
---
2026-08-26 14:48 EDT
15:18
90
16:18
14:48 UTC
45
