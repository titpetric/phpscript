<?php

// @startup

/**
 * Restricts the metadata database to its owner.
 *
 * connection.dsn holds the credentials of every database dbadmin can reach, in
 * cleartext, and the sqlite file is a file like any other. This runs after
 * migrate.php, which sorts before it, so the file exists by the time it is
 * chmodded.
 *
 * The path is asked of the connection rather than read out of the environment:
 * PLATFORM_* variables are infrastructure and are not visible to a script, and
 * a DSN can spell the same file several ways. database_list answers with the
 * path sqlite actually opened, and answers with nothing for a database that is
 * not a local file, which is the other case this has to handle.
 *
 * A permission this cannot set is reported rather than fatal: it is an
 * operator's decision to review, not a reason to refuse to start.
 */

/** metadata_files returns the sqlite files behind $db, including its journals. */
function metadata_files($db) {
	$files = array();

	foreach ($db->get_all("PRAGMA database_list") as $entry) {
		$file = (string)$entry["file"];
		if ($file === "") {
			continue;
		}

		$files[] = $file;
		$files[] = $file . "-wal";
		$files[] = $file . "-shm";
	}

	return $files;
}

/** harden chmods every file the metadata database is stored in. */
function harden($db) {
	$found = false;

	foreach (metadata_files($db) as $path) {
		if (!file_exists($path)) {
			continue;
		}

		$found = true;
		if (!chmod($path, 384)) {
			fwrite(STDERR, "dbadmin: could not restrict " . $path . " to its owner; it holds connection credentials.\n");
		}
	}

	if (!$found) {
		fwrite(STDERR, "dbadmin: the metadata database is not a local file; connection credentials are stored wherever it lives.\n");
	}
}

harden(new Database("dbadmin"));
