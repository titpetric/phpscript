name: the default user store reads the password file at the login, not the new
runner:
  php: false
description: >
  `new Session\Manager` with no arguments takes the application's own
  .htpasswd as its user database. The constructor opens that file and parses
  nothing: a manager constructs on every request, and a page that never calls
  login() does not pay for the account list. This folder's .htpasswd carries a
  plain-text password on its second line, so the manager still constructs and
  the session itself works.

  The parse happens in the call that reads the file. login() refuses the
  broken file whole, with the same message the Auth\Storage\PasswordFile
  constructor gives, naming the line an operator fixes. The path in that
  message is the host's resolved one, so the assertion reads the stable tail.

  The php runner sits this out because Session\Manager is not a PHP name;
  tests/fixtures/bindings/auth_passwordfile_refused.phpt pins the eager half,
  where a script constructing the store by name is refused at the new.
request:
  cookie: {}
---
<?php
$session = new Session\Manager;
$session->start("row-1");
echo $session->get(), "\n";
var_dump($session->valid());

try {
	$session->login("admin", "hunter2hunter2");
	echo "logged in\n";
} catch (Throwable $e) {
	echo str_contains($e->getMessage(), 'line 2: "reader": unreadable password hash')
		? "login refused the file at line 2\n"
		: $e->getMessage();
}
?>
---
row-1
bool(true)
login refused the file at line 2
