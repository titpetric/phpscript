name: password_hash
description: password_hash produces a bcrypt hash password_verify accepts, and PHP's $2y$ revision is what both write.
---
<?php
$cheap = array("cost" => 4);
$hash = password_hash("hunter2", PASSWORD_BCRYPT, $cheap);

echo substr($hash, 0, 4), "\n";
echo strlen($hash), "\n";
echo password_verify("hunter2", $hash) ? "match" : "differ", "\n";
echo password_verify("Hunter2", $hash) ? "match" : "differ", "\n";
echo password_verify("hunter2", "") ? "match" : "differ", "\n";
echo password_verify("hunter2", "not-a-hash") ? "match" : "differ", "\n";

$info = password_get_info($hash);
echo $info["algoName"], " ", $info["options"]["cost"], "\n";

echo password_needs_rehash($hash, PASSWORD_BCRYPT, $cheap) ? "rehash" : "current", "\n";
echo password_needs_rehash($hash, PASSWORD_BCRYPT) ? "rehash" : "current", "\n";

echo PASSWORD_BCRYPT_DEFAULT_COST, "\n";

// Two hashes of the same password differ, because each carries its own salt.
$again = password_hash("hunter2", PASSWORD_BCRYPT, $cheap);
echo ($again === $hash) ? "same" : "salted", "\n";
echo password_verify("hunter2", $again) ? "match" : "differ", "\n";
---
$2y$
60
match
differ
differ
differ
bcrypt 4
current
rehash
12
salted
match
