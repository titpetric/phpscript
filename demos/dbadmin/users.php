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
}

$db->commit();
$db->close();
