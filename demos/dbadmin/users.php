<?php

// @startup

// The user table is a view of .htpasswd, rebuilt on every start.
//
// No migration owns it and schema/ leaves it out: nothing owns a row here
// but the password file. A reload drops and refills the table, which picks up
// a user appended to that file - `@startup` jobs run again on SIGHUP - and the
// database stores no account, no password and no hash of its own.
//
// The id is the primary key other tables carry, and it follows from the
// username, so it survives the rebuild: a user removed from the file loses
// this row and nothing else, and adding the name back restores the same id.
//
// The filename matters. `@startup` files run in path order, and migrate.php
// creates the schema, so this name has to sort after it.
$db = new Database("dbadmin");
$auth = new Auth\Storage\PasswordFile;

if (!$auth->exists()) {
	return;
}

// The ids the table carried before the rebuild. A name the file dropped loses
// its row here, and whoever holds a session under that id loses the session
// with it: this job notices a removal, so this job signs the account out. A
// first start has no table and no rows.
$held = [];
if ($db->get("SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'user'")) {
	foreach ($db->get_all("SELECT id FROM \"user\"") as $row) {
		$held[$row["id"]] = true;
	}
}

// One transaction, because sqlite applies DDL inside one: a request served by
// the previous generation during a reload sees the old table or the new one,
// and never a moment with no table at all.
$db->begin();
$db->query("DROP TABLE IF EXISTS \"user\"");
$db->query("CREATE TABLE \"user\" (id TEXT NOT NULL PRIMARY KEY, username TEXT NOT NULL)");
$db->query("CREATE UNIQUE INDEX uidx_user_username ON \"user\" (username)");

foreach ($auth->users() as $user) {
	// get_id() is the computed ULID, and the record renders as its username
	// in a string context, so the second placeholder takes "$user".
	$db->query("INSERT INTO \"user\" (id, username) VALUES (?, ?)", $user->get_id(), "$user");
	unset($held[$user->get_id()]);
}

$db->commit();
$db->close();

// Whatever remains in $held is an account the file dropped, and every session
// it signs goes now. The session store is the default one, the same disk
// folder the pages read, so a cookie a removed user kept lands on /login.
if ($held) {
	$session = new Session\Manager;
	foreach (array_keys($held) as $id) {
		$session->delete_by_user($id);
	}
}
