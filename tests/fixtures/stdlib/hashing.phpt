name: digest width, collision resistance and what each digest is for
description: >
  What each registered digest is safe to claim. The width table is arithmetic
  over the function's own output, so the birthday bound is derived rather than
  asserted. Three collisions are then exhibited rather than described: the
  Wang-Yu pair that breaks md5 outright, a crc32 pair found by a birthday
  search over 60411 random inputs, and 32-bit truncations of md5, sha1 and
  sha256 that each collide after the same ~2**16 tries, which is the point -
  width buys collision resistance, not the name of the algorithm. The published
  attacks on md5 and sha1 are cited in docs/reference/extensions/hashing.md
  rather than printed here, because an attack complexity is not something this
  fixture can verify. Verified against php 8.5.
---
<?php

// Width is what a generic birthday search has to cover: a collision turns up
// after about 2**(bits/2) inputs. The widths come from the functions, so this
// table is the build's own answer rather than a claim about it.
$algos = array("crc32b", "md5", "sha1", "sha224", "sha256", "sha384", "sha512");
echo "algo     bits  hex  birthday\n";
foreach ($algos as $algo) {
    $hex = strlen(hash($algo, ""));
    echo sprintf("%-9s%4d %4d  2**%d\n", $algo, $hex * 4, $hex, $hex * 2);
}

// md5 does not reach its birthday bound. These are the two 128-byte messages
// Wang and Yu published in 2004; they differ in six bytes and have the same
// md5. Nothing here searched for them, and nothing could: the pair comes from
// a differential attack on md5's compression function.
echo "\n";
$a = hex2bin("d131dd02c5e6eec4693d9a0698aff95c2fcab58712467eab4004583eb8fb7f89" .
    "55ad340609f4b30283e488832571415a085125e8f7cdc99fd91dbdf280373c5b" .
    "d8823e3156348f5bae6dacd436c919c6dd53e2b487da03fd02396306d248cda0" .
    "e99f33420f577ee8ce54b67080a80d1ec69821bcb6a8839396f9652b6ff72a70");
$b = hex2bin("d131dd02c5e6eec4693d9a0698aff95c2fcab50712467eab4004583eb8fb7f89" .
    "55ad340609f4b30283e4888325f1415a085125e8f7cdc99fd91dbd7280373c5b" .
    "d8823e3156348f5bae6dacd436c919c6dd53e23487da03fd02396306d248cda0" .
    "e99f33420f577ee8ce54b67080280d1ec69821bcb6a8839396f965ab6ff72a70");
echo strlen($a), " ", strlen($b), " ", ($a === $b) ? "same" : "differ", "\n";
echo md5($a), "\n";
echo md5($b), "\n";
var_dump(md5($a) === md5($b));

// sha1 survives this particular pair, which says nothing about sha1: its own
// collision needs a 400KB pair that does not belong in a fixture.
var_dump(sha1($a) === sha1($b));

// crc32 is 32 bits wide, so a birthday search covers it on a laptop. This
// pair came out of 60411 random 16-character inputs, against the 2**16 the
// table above predicts. Both spellings of the checksum collide, because they
// are the same function: crc32() returns the integer, hash("crc32b") the hex.
echo "\n";
$x = "102daaee64a84983";
$y = "319a481a369adaa1";
echo crc32($x), " ", crc32($y), "\n";
var_dump(crc32($x) === crc32($y));
var_dump(hash("crc32b", $x) === hash("crc32b", $y));

// A cryptographic digest cut to 32 bits is no better off. Each of these pairs
// was found by the same search, in 17915, 90803 and 94374 tries: the cost
// tracks the width, not the algorithm.
echo "\n";
$truncated = array(
    array("md5", "n551056921-26361235", "n1961471899-75668387"),
    array("sha1", "n1787368608-101629756", "n680251283-125662735"),
    array("sha256", "n604053296-1481701170", "n726875346-672341916"),
);
foreach ($truncated as $case) {
    list($algo, $one, $two) = $case;
    $left = substr(hash($algo, $one), 0, 8);
    $right = substr(hash($algo, $two), 0, 8);
    echo sprintf("%-7s %s %s %s\n", $algo, $left, $right, ($left === $right) ? "collide" : "differ");
}

// crc16 and crc64 are not hash() algorithms. php does not carry them either,
// so a script wanting a short checksum takes crc32 and a script wanting a
// 64-bit one truncates a real digest.
echo "\n";
var_dump(in_array("crc16", hash_algos()));
var_dump(in_array("crc64", hash_algos()));
var_dump(in_array("crc32b", hash_algos()));
var_dump(in_array("sha256", hash_algos()));

// Why a password is not stored as a digest, in two lines: a digest of a
// password is a constant, so one precomputed table answers every account at
// once. password_hash() salts, so the same password twice is two values.
echo "\n";
echo md5("hunter2"), "\n";
echo md5("hunter2"), "\n";
$cheap = array("cost" => 4);
$first = password_hash("hunter2", PASSWORD_BCRYPT, $cheap);
$second = password_hash("hunter2", PASSWORD_BCRYPT, $cheap);
var_dump($first === $second);
var_dump(password_verify("hunter2", $first), password_verify("hunter2", $second));
?>
---
algo     bits  hex  birthday
crc32b     32    8  2**16
md5       128   32  2**64
sha1      160   40  2**80
sha224    224   56  2**112
sha256    256   64  2**128
sha384    384   96  2**192
sha512    512  128  2**256

128 128 differ
79054025255fb1a26e4bc422aef54eb4
79054025255fb1a26e4bc422aef54eb4
bool(true)
bool(false)

1237513312 1237513312
bool(true)
bool(true)

md5     5e01af77 5e01af77 collide
sha1    c1ff38a6 c1ff38a6 collide
sha256  797ca150 797ca150 collide

bool(false)
bool(false)
bool(true)
bool(true)

2ab96390c7dbe3439de74d0c9b0b1767
2ab96390c7dbe3439de74d0c9b0b1767
bool(false)
bool(true)
bool(true)
