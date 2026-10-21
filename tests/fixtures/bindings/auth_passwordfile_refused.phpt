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

  A file that is not there is the one state that is not an error. It is what a
  tree with no user database looks like, exists() is the question for it, and
  Session\Manager takes a default store without that default failing a site
  that has no login at all.
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
foreach (array("broken", "uppercase", "duplicate") as $name) {
	try {
		new Auth\Storage\PasswordFile("passwordfile/" . $name);
		echo "constructed\n";
	} catch (Throwable $e) {
		echo $e->getMessage(), "\n";
	}
}

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
bool(false)
