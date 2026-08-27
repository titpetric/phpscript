name: an RFC 3339 instant round trips through a declared date column
description: >
  sqlite has no date type: the declared column type is what makes the driver
  scan into a Time, and the text stored is what carries the zone. Writing the
  TIME_RFC3339 layout keeps the offset, so the value read back is the instant
  that was written and not a wall clock in some other zone. Needs the
  sqlite_test credential from .env.testing.
runner:
  php: false
---
<?php

$db = new Database("sqlite_test");
$db->query("DROP TABLE IF EXISTS events");
$db->query("CREATE TABLE events (id INTEGER PRIMARY KEY, at TIMESTAMP NOT NULL)");

set_timezone("UTC");
$written = DateTime::parse(TIME_DATETIME, "2026-08-26 14:48:00");

// Format, do not bind the value: a bound Time reaches sqlite as Go's own
// rendering, which sqlite's own date functions cannot read.
$db->query("INSERT INTO events (id, at) VALUES (?, ?)", 1,
	$written->in(new Time\Location("Europe/Ljubljana"))->format(TIME_RFC3339));

// The text sqlite holds keeps the offset it was given.
$stored = $db->get("SELECT CAST(at AS TEXT) AS s FROM events WHERE id = 1");
echo $stored["s"], "\n";

// The declared type makes the driver hand the column back as a Time.
$row = $db->get("SELECT at FROM events WHERE id = 1");
$read = $row["at"];
echo $read->format(TIME_RFC3339), "\n";
echo $read->unix() == $written->unix() ? "same instant\n" : "shifted\n";

// echo prints a wall clock in whichever zone the value carries, which is why
// a comparison reads the instant rather than the text.
echo $read, "\n";
echo $written, "\n";
---
2026-08-26T16:48:00+02:00
2026-08-26T16:48:00+02:00
same instant
2026-08-26 16:48:00
2026-08-26 14:48:00
