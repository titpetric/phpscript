name: a script cannot register a sqlite connection outside the root
description: >
  Database::register takes its DSN from PHP. A relative sqlite path resolves
  against the application root when the connection opens, but an absolute one and
  a file: URI were handed to the driver verbatim, and opening the connection
  creates the file - so a script could put a database anywhere the process could
  write while file_exists() on the same path answered false.

  Refused at registration rather than anchored, so the script sees which spelling
  is wrong. A relative path and a memory DSN are unaffected, and an absolute DSN
  an operator writes in config.yml still resolves as it did: the operator is not
  a tenant, and resolveSQLiteDSN still passes one through.

  The expectation is phpscript's sandbox rather than php's behaviour, so the php
  runner is opted out: php has no root to anchor to and no such class.
runner:
  php: false
---
<?php

echo "absolute: ";
try {
	Database::register("esc", "sqlite:///tmp/phpscript-fixture-escape.sqlite");
	echo "registered\n";
} catch (Throwable $e) {
	echo "refused\n";
}
var_dump(file_exists("/tmp/phpscript-fixture-escape.sqlite"));

echo "file uri: ";
try {
	Database::register("esc2", "sqlite://file:/tmp/phpscript-fixture-escape2.sqlite");
	echo "registered\n";
} catch (Throwable $e) {
	echo "refused\n";
}

echo "relative: ";
try {
	Database::register("rel", "sqlite://data/app.sqlite");
	echo "registered\n";
} catch (Throwable $e) {
	echo "refused\n";
}

echo "memory:   ";
try {
	Database::register("mem", "sqlite://:memory:");
	echo "registered\n";
} catch (Throwable $e) {
	echo "refused\n";
}
---
absolute: refused
bool(false)
file uri: refused
relative: registered
memory:   registered
