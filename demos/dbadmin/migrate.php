<?php

// @startup

/**
 * Applies the schema before the server accepts a request.
 *
 * Migration files are append only: progress is recorded per statement index,
 * so a later change to a table is a new statement at the end of its file, and
 * a new table is a new file.
 */

$migrate = new Database\Migrate("dbadmin");

$migrate->load("./schema/*.up.sql");
$migrate->run();
