name: a password file is the user database a session logs in against
runner:
  php: false
description: >
  Auth\Storage\PasswordFile reads an apache-format password file and answers a
  record: the username as its string form, and a primary key computed from that
  name. The three lines of the file carry one password in three formats, so a
  file written by any htpasswd flag signs in, and a file mid-rewrite from one
  format to another signs in throughout.

  What a script cannot do is read a hash. The store has no property holding
  one, authenticate() answers a record or null, and the file itself is never
  opened in PHP.

  The key is derived and not stored, which is the property other tables rest
  on: the same username answers the same 26-character id on every call and
  after the file is rewritten, so a row carrying it resolves for as long as
  that name exists, and again if it is removed and added back.

  The php runner sits this out because neither class is a PHP name: php has no
  user database of its own and htpasswd is the web server's business there. The
  formats themselves are checked against php in
  tests/fixtures/stdlib/password_file_formats.phpt, which uses php's own
  functions on the same hashes, and the ids below are the Crockford base32 of
  the first 128 bits of sha256(username), computed in php rather than read off
  this runtime.
request:
  cookie: {}
---
<?php
$users = new Auth\Storage\PasswordFile("passwordfile/.htpasswd");
var_dump($users->exists());

// One password, three storage formats, one answer each.
foreach (array("admin", "apache", "legacy") as $name) {
	$user = $users->authenticate($name, "hunter2hunter2");
	echo $name, " ", strlen($user->get_id()), " ", $user, "\n";
}

// A record renders as its username with nothing spelled at the call site:
// string interpolation, concatenation and a string parameter all read it.
$user = $users->authenticate("admin", "hunter2hunter2");
echo "signed in as $user, key " . $user->get_id() . "\n";
echo strtoupper($user), "\n";

// The key is a function of the name alone, so it is the same on every call and
// differs between users.
var_dump($user->get_id() === $users->authenticate("admin", "hunter2hunter2")->get_id());
var_dump($user->get_id() === $users->authenticate("apache", "hunter2hunter2")->get_id());

// A wrong password, an unknown user and a username in the wrong case are one
// answer, so nothing a page prints says which of the three it was.
var_dump($users->authenticate("admin", "hunter3hunter3"));
var_dump($users->authenticate("nobody", "hunter2hunter2"));
var_dump($users->authenticate("Admin", "hunter2hunter2"));

// The listing is the file as a table of two columns. It is ordered by
// username, and no hash is in it.
foreach ($users->users() as $row) {
	echo $row->get_id(), " ", $row, "\n";
}

// The session is what turns one of those records into a login. It stores the
// user's key, so a row written during the session references the user the way
// any other table does.
$session = new Session\Manager(new Session\Storage\Memory, $users);
var_dump($session->login("admin", "hunter2hunter2"));
echo $session->get(), "\n";
var_dump($session->valid());

// A refused login leaves the session alone.
var_dump($session->login("admin", "wrong"));
?>
---
bool(true)
admin 26 admin
apache 26 apache
legacy 26 legacy
signed in as admin, key 4CD5VEBDA10GAVVT88QN6YW5EZ
ADMIN
bool(true)
bool(false)
NULL
NULL
NULL
4CD5VEBDA10GAVVT88QN6YW5EZ admin
6QW16BMTMBM54CJSM60RSEYDHP apache
64KZN789FTFY39K2BTJZ0NKHK9 legacy
bool(true)
4CD5VEBDA10GAVVT88QN6YW5EZ
bool(true)
bool(false)
