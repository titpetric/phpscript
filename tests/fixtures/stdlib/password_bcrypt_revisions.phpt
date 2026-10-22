name: the bcrypt revision markers password_verify reads
description: >
  A bcrypt hash spells its revision as `$2y$`, `$2a$` or `$2b$`, and the three
  name the same derivation of the same body: php's crypt accepts any of them,
  and x/crypto parses the minor letter and carries it through the comparison,
  so the revision cancels and no rewrite sits between a stored hash and the
  verify. This fixture is one php-written hash under all three spellings, so
  the expected output is php's and a divergence on any marker fails the php
  column.

  The hash was written by php:

    php -r 'echo password_hash("hunter2", PASSWORD_BCRYPT, ["cost" => 4]);'

  The cost is the lowest bcrypt accepts, because what is under test is which
  revision verifies and not what a derivation costs.
---
<?php
$hash = '$2y$04$WmT0g666saxPq75RDYYNtOIhBRHjNejxJUmMZRbdM1dNyrUKxKFBK';

foreach (array('$2y$', '$2a$', '$2b$') as $revision) {
	$stored = $revision . substr($hash, 4);
	echo $revision, " ";
	var_dump(password_verify("hunter2", $stored));
	echo $revision, " wrong ";
	var_dump(password_verify("hunter3", $stored));
}
---
$2y$ bool(true)
$2y$ wrong bool(false)
$2a$ bool(true)
$2a$ wrong bool(false)
$2b$ bool(true)
$2b$ wrong bool(false)
