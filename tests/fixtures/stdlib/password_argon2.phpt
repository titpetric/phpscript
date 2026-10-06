name: password_hash with argon2id and argon2i
description: >
  The argon2 variants under php's own identifiers and option names. Covers the
  encoded shape, the round trip, what password_get_info reads back and in which
  order, password_needs_rehash against a different parameter set, and the two
  pasted hashes php 8.5 wrote, which this runtime has to verify for a stored
  table to survive moving between the two. The parameters are deliberately tiny
  so the fixture costs a millisecond; the ones to deploy are in
  docs/reference/extensions/hashing.md. Verified against php 8.5.
---
<?php

$cheap = array("memory_cost" => 256, "time_cost" => 1, "threads" => 1);

echo implode(",", password_algos()), "\n";
echo PASSWORD_ARGON2ID, " ", PASSWORD_ARGON2I, "\n";
echo PASSWORD_ARGON2_DEFAULT_MEMORY_COST, " ", PASSWORD_ARGON2_DEFAULT_TIME_COST, " ", PASSWORD_ARGON2_DEFAULT_THREADS, "\n";

// The encoded form is the PHC string: the variant, the version, the three work
// factors, the salt and the tag. Its length is fixed once the factors are.
echo "\n";
$hash = password_hash("hunter2", PASSWORD_ARGON2ID, $cheap);
echo substr($hash, 0, 29), "\n";
echo strlen($hash), "\n";
echo password_verify("hunter2", $hash) ? "match" : "differ", "\n";
echo password_verify("Hunter2", $hash) ? "match" : "differ", "\n";
echo password_verify("hunter2", "not-a-hash") ? "match" : "differ", "\n";

// The salt is fresh per call, so one password is two stored values and both
// verify.
echo "\n";
$again = password_hash("hunter2", PASSWORD_ARGON2ID, $cheap);
echo ($again === $hash) ? "same" : "salted", "\n";
echo password_verify("hunter2", $again) ? "match" : "differ", "\n";

// password_get_info reads the variant and the factors back out, under the same
// key names the options took and in the same order.
echo "\n";
$info = password_get_info($hash);
echo $info["algo"], " ", $info["algoName"], "\n";
echo implode(",", array_keys($info["options"])), "\n";
echo $info["options"]["memory_cost"], " ", $info["options"]["time_cost"], " ", $info["options"]["threads"], "\n";

// A different algorithm or a different factor is a rehash; the same ones are
// not. This is the upgrade path off bcrypt, taken on a successful login.
echo "\n";
echo password_needs_rehash($hash, PASSWORD_ARGON2ID, $cheap) ? "rehash" : "current", "\n";
echo password_needs_rehash($hash, PASSWORD_ARGON2ID) ? "rehash" : "current", "\n";
echo password_needs_rehash($hash, PASSWORD_BCRYPT) ? "rehash" : "current", "\n";
$bcrypt = password_hash("hunter2", PASSWORD_BCRYPT, array("cost" => 4));
echo password_needs_rehash($bcrypt, PASSWORD_ARGON2ID, $cheap) ? "rehash" : "current", "\n";
echo password_verify("hunter2", $bcrypt) ? "match" : "differ", "\n";

// argon2i is the data-independent variant and is a different derivation, so a
// tag written by one does not verify as the other.
echo "\n";
$i = password_hash("hunter2", PASSWORD_ARGON2I, $cheap);
echo substr($i, 0, 28), "\n";
echo password_verify("hunter2", $i) ? "match" : "differ", "\n";
echo password_get_info($i)["algoName"], "\n";
echo password_verify("hunter2", str_replace('argon2i$', 'argon2id$', $i)) ? "match" : "differ", "\n";

// Two hashes php 8.5 wrote, pasted. A stored table has to keep working when
// the runtime under it changes, which is the only thing that makes
// implementing php's function rather than exposing argon2 worth doing.
echo "\n";
$fromPHP = array(
    '$argon2id$v=19$m=256,t=3,p=2$d3ZTWk9yMk16Y0t4SkgvYQ$f+VzAc7Ya5B04A2ZJWeJCPJ5jkZZFRaUTXxR982VaN8',
    '$argon2i$v=19$m=256,t=1,p=1$cS9QSzJUc0FEdG9GMHA2Wg$DqXqGB6ym/9L5u2JTWBA/tIjewj0KUPJOnJgpw5WdK4',
);
foreach ($fromPHP as $stored) {
    $info = password_get_info($stored);
    echo $info["algoName"], " ",
        $info["options"]["memory_cost"], " ",
        $info["options"]["time_cost"], " ",
        $info["options"]["threads"], " ",
        password_verify("pw", $stored) ? "match" : "differ", " ",
        password_verify("px", $stored) ? "match" : "differ", "\n";
}
?>
---
2y,argon2i,argon2id
argon2id argon2i
65536 4 1

$argon2id$v=19$m=256,t=1,p=1$
95
match
differ
differ

salted
match

argon2id argon2id
memory_cost,time_cost,threads
256 1 1

current
rehash
rehash
rehash
match

$argon2i$v=19$m=256,t=1,p=1$
match
argon2i
differ

argon2id 256 3 2 match differ
argon2i 256 1 1 match differ
