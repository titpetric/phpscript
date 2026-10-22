name: a password file is refused whole, at the line that is wrong
runner:
  php: false
description: >
  Everything a password file can get wrong is refused by the constructor, with
  the line it is on, rather than by the login it would have broken. A file half
  of which authenticates is worse than one that does not load: the half that
  does not is discovered by the user it locks out.

  Plain text is refused rather than read, which is a decision. Apache reads a
  plain password on some platforms, and a line nothing else claims is also what
  a truncated or corrupted hash looks like, so accepting the class would turn a
  damaged file into one that authenticates. php's `$1$` md5-crypt gets its own
  sentence because it is one character away from apache's `$apr1$` and is the
  mistake a port makes.

  A username that is not lowercase is refused for a reason the format does not
  have: the primary key is computed from the name, so two spellings would be
  two users with two keys and two sets of rows.

  A utf-8 byte order mark is refused for the account it would hide: TrimSpace
  leaves the mark on the first username, so that user matches nothing a login
  types and nobody learns why. A bcrypt line with no revision marker - the
  1997 "$2$" spelling - is refused because php's crypt answers the failure
  string "*0" for it, so the line would sign in here and nowhere else;
  tests/fixtures/stdlib/password_bcrypt_revisions.phpt settles that against
  the php binary.

  A file that is not there is the one state that is not an error. It is what a
  tree with no user database looks like, exists() is the question for it, and
  Session\Manager takes a default store without that default failing a site
  that has no login at all. A file of comments alone constructs the same way,
  and exists() answers false for it too: there is no account to ask about, and
  a login page that printed "wrong password" over that state would send the
  operator reading the wrong file.
---
<?php
// The file that is not there: a store, not a refusal.
$absent = new Auth\Storage\PasswordFile("passwordfile/nothing-here");
var_dump($absent->exists());
var_dump($absent->users());

// Reading one is still an error, because a login that cannot be answered is a
// misconfiguration and not a wrong password.
try {
	$absent->authenticate("admin", "hunter2hunter2");
} catch (Throwable $e) {
	echo "no file\n";
}

// Each of these names the line, so the message is what an operator fixes.
foreach (array("broken", "uppercase", "duplicate", "bom", "no-revision") as $name) {
	try {
		new Auth\Storage\PasswordFile("passwordfile/" . $name);
		echo "constructed\n";
	} catch (Throwable $e) {
		echo $e->getMessage(), "\n";
	}
}

// A file of comments alone is a valid file holding no accounts: it
// constructs, and exists() answers false the way a missing file does.
$staged = new Auth\Storage\PasswordFile("passwordfile/comments");
var_dump($staged->exists());
var_dump($staged->users());

// A path cannot climb out of the application root: the spelling below resolves
// to the fixture's own directory, where no such file exists.
$outside = new Auth\Storage\PasswordFile("../../../../etc/shadow");
var_dump($outside->exists());
?>
---
bool(false)
array(0) {
}
no file
Auth\Storage\PasswordFile: passwordfile/broken: line 2: "reader": unreadable password hash, want bcrypt ($2y$), argon2, apache md5 ($apr1$) or {SHA}; write it with htpasswd -B
Auth\Storage\PasswordFile: passwordfile/uppercase: line 2: username "Reader" is not lowercase; the primary key is computed from the name, so two spellings are two users
Auth\Storage\PasswordFile: passwordfile/duplicate: line 2: "admin" is already in this file
Auth\Storage\PasswordFile: passwordfile/bom: line 1: a utf-8 byte order mark precedes the first username; save the file without one
Auth\Storage\PasswordFile: passwordfile/no-revision: line 1: "admin": unreadable password hash, want bcrypt ($2y$), argon2, apache md5 ($apr1$) or {SHA}; write it with htpasswd -B
bool(false)
array(0) {
}
bool(false)
