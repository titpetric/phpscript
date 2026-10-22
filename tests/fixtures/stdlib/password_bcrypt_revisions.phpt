name: the bcrypt revision markers password_verify reads, settled by php
description: >
  php's crypt reads four bcrypt revision markers - $2a$, $2b$, $2x$ and $2y$ -
  and answers the failure string "*0" for anything else under the $2 prefix,
  the 1997 "$2$" spelling with no minor revision included. x/crypto parses
  "$2$" and any minor byte, so without a pin this runtime verified two
  spellings php refuses, and a password file could carry a line that signed in
  here and nowhere else.

  The expected output below is php's own, from running this body through the
  php binary. The body of every hash is one php-written derivation, restored
  under each marker; the markers agree on the digest because the revisions
  differ only in how a password carrying 8-bit bytes feeds the key schedule,
  and this one is ASCII.

  password_get_info is in here for the two refused spellings alone: php
  reports no algorithm for them, and a runtime that still read a cost off
  "$2$" would say bcrypt where php says unknown. The markers php verifies but
  does not report - $2a$, $2b$, $2x$ answer "unknown" from password_get_info
  in php - stay out, because this runtime reports them as bcrypt and that
  divergence is not this fixture's subject.
---
<?php

// One php-written bcrypt body, restored under every revision marker. The salt
// and the tag come from:
//
//   php -r 'echo password_hash("hunter2hunter2", PASSWORD_BCRYPT, ["cost" => 4]);'
$body = '04$j1zOEFHUWgLb8D1SytMeoeFw/s1.z5eOLow3m0r25ypMopsjEbQGW';

// The four markers php's crypt reads. The same password verifies under each,
// because the derivation differs only for passwords carrying 8-bit bytes.
foreach (array('2y', '2a', '2b', '2x') as $rev) {
	echo $rev, ' ';
	var_dump(password_verify('hunter2hunter2', '$' . $rev . '$' . $body));
}

// The 1997 spelling with no minor revision, then a marker crypt never wrote.
// Both read as no hash at all: verify answers false and get_info reports no
// algorithm, in both runtimes.
foreach (array('$2$', '$2c$') as $prefix) {
	echo $prefix, ' ';
	var_dump(password_verify('hunter2hunter2', $prefix . $body));
	$info = password_get_info($prefix . $body);
	echo $prefix, ' ', $info['algoName'], "\n";
}
?>
---
2y bool(true)
2a bool(true)
2b bool(true)
2x bool(true)
$2$ bool(false)
$2$ unknown
$2c$ bool(false)
$2c$ unknown
