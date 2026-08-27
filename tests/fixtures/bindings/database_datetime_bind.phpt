name: a lone Time argument binds by position
description: >
  A statement binding exactly one non-scalar value is read as a set of named
  parameters, which is what makes insert() work. A Time is exempt: it is a
  value the driver binds on its own, and a date column is written one column
  at a time often enough that the exemption has to hold. Needs pdo v0.2.4 and
  the sqlite_test credential from .env.testing.
runner:
  php: false
---
<?php

$db = new Database("sqlite_test");
$db->query("DROP TABLE IF EXISTS appointment");
$db->query("CREATE TABLE appointment (at TIMESTAMP NOT NULL)");

set_timezone("UTC");
$written = DateTime::parse(TIME_DATETIME, "2026-08-26 14:48:00");

// One placeholder, one bound value, and that value is a Time. Before the
// exemption this reached the server with its ? unbound, as a syntax error.
$db->query("INSERT INTO appointment (at) VALUES (?)", $written);

$read = $db->get("SELECT at FROM appointment")["at"];
echo $read->format(TIME_RFC3339), "\n";
echo $read->unix() == $written->unix() ? "same instant\n" : "shifted\n";
---
2026-08-26T14:48:00Z
same instant
