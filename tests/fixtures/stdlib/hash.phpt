name: hash, hash_hmac and hash_equals
description: >
  The algorithm-agnostic digests, against the vectors php answers for them.
  Covers every name this build carries, the raw-bytes form, HMAC under a short
  and an over-long key, and the length-first reading of hash_equals. The
  algorithm list is this build's own and shorter than php's, so the fixture
  names the algorithms rather than printing hash_algos().
---
<?php

$algos = array("md5", "sha1", "sha224", "sha256", "sha384", "sha512", "crc32b");
foreach ($algos as $algo) {
    echo str_pad($algo, 7), " ", hash($algo, "hello"), "\n";
}

// The empty string is a digest like any other, and the answer is the
// algorithm's well-known constant rather than an empty one.
echo "\n";
echo hash("sha256", ""), "\n";

// $binary asks for the raw bytes. They arrive as an ordinary string, so
// strlen counts them and bin2hex spells them back.
echo "\n";
$raw = hash("sha256", "hello", true);
echo strlen($raw), " ", bin2hex($raw), "\n";

// HMAC under a short key, and under a key longer than the block size - which
// HMAC digests first, so the answer is not the same as hashing the long key
// by hand and using that.
echo "\n";
echo hash_hmac("sha256", "hello", "key"), "\n";
echo hash_hmac("md5", "hello", "key"), "\n";
echo hash_hmac("sha256", "hello", str_repeat("k", 200)), "\n";
echo bin2hex(hash_hmac("sha256", "hello", "key", true)), "\n";

// hash_equals compares the length first, so a guess of the wrong length is
// false without looking at a byte of it.
echo "\n";
var_dump(hash_equals("abc", "abc"));
var_dump(hash_equals("abc", "abd"));
var_dump(hash_equals("abc", "ab"));
var_dump(hash_equals("", ""));

// The shape the API-token lookup needs: a digest of the secret, compared
// against the digest a row carries.
echo "\n";
$secret = "01TESTTOKEN";
$stored = hash("sha256", $secret);
var_dump(hash_equals($stored, hash("sha256", $secret)));
var_dump(hash_equals($stored, hash("sha256", $secret . "x")));
?>
---
md5     5d41402abc4b2a76b9719d911017c592
sha1    aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d
sha224  ea09ae9cc6768c50fcee903ed054556e5bfc8347907f12598aa24193
sha256  2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
sha384  59e1748777448c69de6b800d7a33bbfb9ff1b463e44354c3553bcdb9c666fa90125a3c79f90397bdf5f6a13de828684f
sha512  9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043
crc32b  3610a686

e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855

32 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824

9307b3b915efb5171ff14d8cb55fbcc798c6c0ef1456d66ded1a6aa723a58b7b
04130747afca4d79e32e87cf2104f087
174c64caec6f5e924a5df231d79fbb52a10cd73d1b575ded749f9a8ab81c4886
9307b3b915efb5171ff14d8cb55fbcc798c6c0ef1456d66ded1a6aa723a58b7b

bool(true)
bool(false)
bool(false)
bool(true)

bool(true)
bool(false)
