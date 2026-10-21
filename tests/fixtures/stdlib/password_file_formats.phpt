name: the formats a password file carries, read with php's own functions
description: >
  Auth\Storage\PasswordFile reads four stored formats, and three of them have a
  php spelling: bcrypt and argon2 through password_verify, and {SHA} as one
  sha1 compared with hash_equals. This fixture is those spellings on the same
  hashes the password-file fixtures use, so the expected output is php's and
  the formats are settled by php rather than by this runtime.

  It also pins where the responsibility stops. password_verify() answers false
  for `$apr1$` and for `{SHA}` in php, and must answer false here too: apache's
  md5-crypt is read by the user database and not by the password_hash family,
  and a function that started guessing at other formats would diverge from php
  on a value a script can see.

  php cannot derive `$apr1$` at all - `crypt($pw, '$apr1$Zx1kLm9q$')` answers
  the failure string `*0` - so there is no php spelling of that format to
  compare against, and no assertion here for one. Its vectors come from
  `openssl passwd -apr1` instead, in stdlib/crypto/apr1_test.go. `crypt()`
  itself is not registered in this runtime, which is why the comparison is not
  written here as a divergence either.

  The hashes were written by php and openssl:

    php -r 'echo password_hash("hunter2hunter2", PASSWORD_BCRYPT, ["cost" => 4]);'
    php -r 'echo password_hash("hunter2hunter2", PASSWORD_ARGON2ID, ["memory_cost" => 8, "time_cost" => 1, "threads" => 1]);'
    php -r 'echo "{SHA}" . base64_encode(sha1("hunter2hunter2", true));'
    openssl passwd -apr1 -salt Zx1kLm9q hunter2hunter2

  The work factors are the lowest each algorithm accepts, because what is under
  test is which format verifies and not what a derivation costs.
---
<?php

// One password, stored the four ways a password file can carry it.
$password = "hunter2hunter2";
$stored = array(
	"bcrypt" => '$2y$04$j1zOEFHUWgLb8D1SytMeoeFw/s1.z5eOLow3m0r25ypMopsjEbQGW',
	"argon2id" => '$argon2id$v=19$m=8,t=1,p=1$cy9CN0dXU2gvYnVVemZmRw$bJcfAH11Sej7T96VdH4lBTbzdCH4FhajmuGb8ZYGRh0',
	"sha" => '{SHA}/IxesZSAbjGiE/BzEx5zsAEqD7U=',
	"apr1" => '$apr1$Zx1kLm9q$BwLItAYS8lPw/bCsIXHax1',
);

// The two formats password_verify() reads. A file carrying either is read by
// the same derivation a script would use, in both runtimes.
foreach (array("bcrypt", "argon2id") as $format) {
	echo $format, " ";
	var_dump(password_verify($password, $stored[$format]));
	echo $format, " wrong ";
	var_dump(password_verify("hunter3hunter3", $stored[$format]));
}

// What each stored value records about itself.
foreach (array("bcrypt", "argon2id") as $format) {
	$info = password_get_info($stored[$format]);
	echo $format, " ", $info["algoName"], " ";
	print_r($info["options"]);
}

// {SHA} is one sha1 of the password, base64 encoded, and nothing else: no
// salt, no cost parameter, so it is a wordlist lookup per row and the whole of
// what verifying it costs is one digest.
echo "sha ";
var_dump(hash_equals($stored["sha"], "{SHA}" . base64_encode(sha1($password, true))));
echo "sha wrong ";
var_dump(hash_equals($stored["sha"], "{SHA}" . base64_encode(sha1("hunter3hunter3", true))));

// Neither apache format is password_verify()'s business, in either runtime:
// $apr1$ is apache's md5-crypt and {SHA} is its own encoding, so the function
// that reads bcrypt and argon2 answers false for both rather than guessing.
echo "apr1 through password_verify ";
var_dump(password_verify($password, $stored["apr1"]));
echo "sha through password_verify ";
var_dump(password_verify($password, $stored["sha"]));
?>
---
bcrypt bool(true)
bcrypt wrong bool(false)
argon2id bool(true)
argon2id wrong bool(false)
bcrypt bcrypt Array
(
    [cost] => 4
)
argon2id argon2id Array
(
    [memory_cost] => 8
    [time_cost] => 1
    [threads] => 1
)
sha bool(true)
sha wrong bool(false)
apr1 through password_verify bool(false)
sha through password_verify bool(false)
