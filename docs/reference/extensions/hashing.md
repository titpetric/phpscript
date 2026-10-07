# Hashing

| Function                                       | Width               | Job                                                   |
|------------------------------------------------|---------------------|-------------------------------------------------------|
| `crc32`, `hash("crc32b", ...)`                 | 32 bits             | Detect accidental corruption                          |
| `md5`                                          | 128 bits            | Fingerprint data nobody is attacking                  |
| `sha1`                                         | 160 bits            | Fingerprint data nobody is attacking                  |
| `hash("sha224", ...)` to `hash("sha512", ...)` | 224 to 512 bits     | Fingerprint data somebody may try to forge            |
| `hash_hmac`                                    | width of its digest | Authenticate data under a shared key                  |
| `hash_equals`                                  | n/a                 | Compare two digests without leaking where they differ |
| `password_hash`, `password_verify`             | n/a                 | Store a password                                      |

`hash_algos()` answers the seven names this build carries: `md5`, `sha1`, `sha224`, `sha256`, `sha384`, `sha512` and `crc32b`. The list is a seventh of php's, so a script that offers the choice reads it rather than assuming one. `password_algos()` answers the three php answers, in php's order: `2y`, `argon2i` and `argon2id`.

This page is about the functions: what each one's output is worth, and which of them answers which question. The wider security posture they sit inside - secrets, key handling, what a vhost is allowed to reach - is issue #139's remaining scope and is not documented here.

## Four jobs, one word

"Hash" names four unrelated contracts, and a function that satisfies one of them satisfies none of the others.

**A checksum** detects accidental corruption. `crc32` is one: 32 bits, a few cycles per byte, and no claim at all against someone who wants two inputs to agree. CRC-32 is linear over GF(2), so an attacker who can append four bytes to a message sets its checksum to any value they like, in constant time and with no search. It is the right function for a transport check, a cache bucket or an ETag over content you produced yourself. It is never the right function for anything an attacker chooses.

**A message digest** claims collision resistance: that nobody can produce two inputs with the same output. This is the claim md5 and sha1 no longer hold, and `sha256` upward do.

**A keyed digest** claims authenticity: that nobody without the key can produce a valid tag. `hash_hmac` is one. It is what a signed cookie, a webhook signature and a bearer-token tag are built from, and it is the one in this list a request path can afford to run per call.

**A password hash** claims that the stored value is expensive to invert even though the input has little entropy. `password_hash` is one, and it is the only one here. No digest from the rows above is a substitute, for the reason in [Passwords are not digests](#passwords-are-not-digests).

## What width buys

A generic collision search is a birthday search, so it finds a pair after about `2**(bits/2)` inputs. That is arithmetic over the output width and nothing else:

| Algorithm | Bits | Hex characters | Generic collision search |
|-----------|-----:|---------------:|-------------------------:|
| `crc32b`  |   32 |              8 |                  `2**16` |
| `md5`     |  128 |             32 |                  `2**64` |
| `sha1`    |  160 |             40 |                  `2**80` |
| `sha224`  |  224 |             56 |                 `2**112` |
| `sha256`  |  256 |             64 |                 `2**128` |
| `sha384`  |  384 |             96 |                 `2**192` |
| `sha512`  |  512 |            128 |                 `2**256` |

[tests/fixtures/stdlib/hashing.phpt](../../../tests/fixtures/stdlib/hashing.phpt) derives that table from the functions themselves and then exhibits the left-hand end of it: a `crc32` collision found by a birthday search over 60411 random inputs, against the `2**16` the row predicts. The same fixture truncates md5, sha1 and sha256 to 32 bits and collides each of them in the same number of tries. Width decides the cost of the search; the name of the algorithm does not.

`2**128` is the current floor for a digest that has to resist collisions, which is `sha256`. Going wider costs throughput and buys nothing anybody can use.

## md5 and sha1 are broken, and crc32 was never trying

Both md5 and sha1 fall far short of their birthday bound, by published attacks rather than by argument:

| Algorithm | Attack                     | Published                                     | Work    |
|-----------|----------------------------|-----------------------------------------------|--------:|
| md5       | Identical-prefix collision | Wang and Yu, EUROCRYPT 2005                   | `2**39` |
| md5       | Identical-prefix collision | Klima 2006, Stevens 2006                      | `2**24` |
| md5       | Chosen-prefix collision    | Stevens, Sotirov et al., rogue CA, 2008       | `2**50` |
| sha1      | Identical-prefix collision | SHAttered, Stevens et al., 2017               | `2**63` |
| sha1      | Chosen-prefix collision    | SHA-1 is a Shambles, Leurent and Peyrin, 2020 | `2**63` |

The md5 figure is the one that matters for how the function should be read today: a collision is seconds of laptop time, and the fixture carries the Wang-Yu pair as two 128-byte messages with one md5 between them. The sha1 chosen-prefix attack cost about 45,000 US dollars of rented GPU in 2020, which is what removed sha1 from certificates and from PGP key identity.

Preimage resistance of both is unbroken: nobody can produce an input for a given md5. That is not a reason to keep using either, and it is specifically not a reason to store a password as one, because a password is guessed rather than inverted.

What md5 and sha1 are still fine for is everything with no adversary in it: a cache key, a change detector over your own files, a shard selector, an ETag. What they are not fine for is any value an attacker can influence and then benefit from duplicating: a content address, a deduplication key over uploads, a commitment, a signature input, a password.

`crc32` is in a different category again. It is not a weakened hash function; it is not a hash function. Treat it as an error detector and nothing else.

## crc16 and crc64 do not exist here, or in php

Neither name is in php's `hash_algos()`, so there is no behaviour to be compatible with and neither is registered. A script wanting a short checksum takes `crc32`. A script wanting a 64-bit one truncates a real digest, which is what `substr(hash("sha256", $data), 0, 16)` does and is sound: the truncated forms of SHA-2 are defined exactly that way, and 64 bits of output is a `2**32` collision search, which is not enough for anything adversarial.

## What this build does not carry

php's `hash_algos()` has sixty entries. The gap is a gap, not a decision:

| Absent                                   | What it would be for                                                                                                                            |
|------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------|
| `sha3-256`, `sha3-512`                   | A sponge construction with no length-extension property. Go has `crypto/sha3`.                                                                  |
| `sha512/256`                             | SHA-2 on 64-bit words truncated to 256 bits: faster than `sha256` on a 64-bit CPU and not length-extendable. Go has `crypto/sha512.Sum512_256`. |
| `xxh3`, `xxh64`                          | A fast non-cryptographic digest, which is what a cache key actually wants.                                                                      |
| `hash_pbkdf2`                            | A key derived from a password or a shared secret, with an iteration count.                                                                      |
| `hash_init`, `hash_update`, `hash_final` | Digesting a stream without holding it in memory.                                                                                                |
| `hash_file`, `hash_hmac_file`            | The same over a path.                                                                                                                           |

Length extension is the one of these a script can hit by accident. `hash("sha256", $secret . $message)` is forgeable by anyone who knows the length of `$secret`: SHA-2 lets them continue the digest without the secret. `hash_hmac("sha256", $message, $secret)` is the function that is not, and it is registered.

## Passwords are not digests

A digest is fast, and that is the whole problem. An attacker with a stolen table does not invert the digest, they guess: they run the same function over a wordlist and compare. A salt stops one precomputed table from answering every row at once, and it does nothing at all about the guessing rate. `sha256` over a short input is 84 ns here, so the attacker spends 84 ns per guess exactly as the server does; the ratio between what a login costs and what a guess costs is 1 to 1 for any unkeyed digest, at any width.

So there is no bit width that makes a bare digest safe for a password. `sha512` is not safer than `sha256` here; both are the wrong function, and the attribute that makes them wrong is their speed, which no parameter of theirs changes. A salted digest is a weaker store than the one below at the same latency, not a faster one at the same strength.

What makes a password hash a password hash is a **cost parameter**: a number that multiplies the work per guess, set so that one derivation is affordable for the one login a user performs and unaffordable for the billions an attacker needs. `password_hash` has one, and `PASSWORD_BCRYPT_DEFAULT_COST` is its default:

```php
$stored = password_hash($password, PASSWORD_DEFAULT);
if (password_verify($submitted, $stored)) {
    // ...
}
if (password_needs_rehash($stored, PASSWORD_DEFAULT)) {
    $stored = password_hash($submitted, PASSWORD_DEFAULT);
}
```

The cost is recorded inside the hash, so `password_verify` costs what the hash was written at and `password_needs_rehash` is how a deployment moves its stored hashes to a new one. A login is the only place in a request that pays it.

**Where the fast digests belong in an authentication flow** is the token, not the password. A session identifier or a bearer token is 128 bits from `random_bytes`, so there is nothing in it to guess and nothing for a cost parameter to defend; a digest of it is the correct thing to store, and `hash_equals` is the correct way to compare. That is what makes the per-request cost of an authenticated call a digest rather than a key derivation:

```php
// Registration and login: once per user, pays the cost parameter.
$stored = password_hash($password, PASSWORD_DEFAULT);

// Every request after it: a digest over a high-entropy token.
$token  = bin2hex(random_bytes(16));
$lookup = hash("sha256", $token);
if (hash_equals($row["token_hash"], hash("sha256", $presented))) {
    // ...
}
```

A deployment whose login latency is a problem has three levers, in this order: issue a token so the derivation happens once per session rather than once per request; tune the algorithm and its parameters against a measurement on the hardware that will run it; and bound how many derivations can be in flight at once, because each one holds a core for its whole duration.

## What a derivation costs

All numbers from [stdlib/crypto/password_test.go](../../../stdlib/crypto/password_test.go) on an Intel N150, Go 1.27, `CGO_ENABLED=0`, `GOMEMLIMIT=2GiB`, one pinned core: `benchstat` medians over `-count 6` at `-benchtime=100ms`. `password_verify` costs what `password_hash` costs at the same parameters, because it is the same derivation, so one column covers both.

| Algorithm and parameters         | sec/op   | B/op     | allocs/op |
|----------------------------------|---------:|---------:|----------:|
| `bcrypt`, cost 4                 | 898.0 us | 5.03 KiB |         9 |
| `bcrypt`, cost 8                 | 13.44 ms | 5.05 KiB |         9 |
| `bcrypt`, cost 10                | 53.12 ms | 5.11 KiB |        10 |
| `bcrypt`, cost 11                | 107.9 ms | 5.18 KiB |        11 |
| `bcrypt`, cost 12 (the default)  | 214.7 ms | 5.18 KiB |        11 |
| `bcrypt`, cost 13                | 431.4 ms | 5.18 KiB |        11 |
| `argon2id`, m=65536, t=4 (php's) | 204.0 ms | 64.0 MiB |        48 |
| `argon2id`, m=19456, t=2         | 29.65 ms | 19.0 MiB |        32 |
| `argon2id`, m=9216, t=4          | 26.07 ms | 9.00 MiB |        48 |
| `sha256` over a token            | 84.03 ns |        0 |         0 |
| `hash_hmac` sha256 over a token  | 604.4 ns |      512 |         6 |

### How many logins that is

One derivation divided into the core count is the wrong arithmetic. A bcrypt derivation holds a core and 5 KiB, so concurrent ones scale with cores; an argon2id derivation holds 19 MiB and streams it, so they contend for memory bandwidth and scale less. `BenchmarkPasswordVerifyThroughput` measures the rate instead, under `-cpu 1,2,4`: `sec/op` is wall clock per completed login across the cores in play, so logins per second is its reciprocal.

| Stored at               | 1 core   | 2 cores  | 4 cores  | Logins/sec on 4 cores | Per day |
|-------------------------|---------:|---------:|---------:|----------------------:|--------:|
| `bcrypt` cost 12        | 233.2 ms | 137.4 ms | 69.43 ms |                  14.4 |   1.24M |
| `argon2id` m=19456, t=2 | 29.16 ms | 17.70 ms | 13.04 ms |                  76.7 |   6.63M |

bcrypt scales 3.4x over four cores and argon2id 2.2x, which is the bandwidth bound showing up. The ratio that matters is the last column: the same four cores serve 5.3 times as many logins on argon2id, and one login takes 29.16 ms rather than 233.2 ms.

Two more things to read off the cost table.

**Cost 13 is double cost 12 and cost 11 is half.** The parameter is a power of two and the latency follows it exactly, so lowering it is the only bcrypt lever and each step down halves the work an attacker does too.

**The token path is five to six orders of magnitude cheaper.** 214.7 ms against 604.4 ns is a factor of 355,000. Every request that re-derives a password hash instead of checking a token is paying that factor, and no choice of algorithm recovers it.

**argon2id buys the trade the cost parameter cannot.** At RFC 9106's second recommended parameter set, m=19456 KiB and t=2, a derivation is 29.65 ms: seven times less wall clock than bcrypt at cost 12, with a 19 MiB working set per guess against bcrypt's 4 KiB. The memory is the point. bcrypt's working set fits in a GPU core's local memory, so an attacker runs thousands of guesses in parallel on one card; argon2id's does not, and the parallelism an attacker can buy is bounded by memory bandwidth rather than by arithmetic. Lower wall clock and higher attacker cost at the same time is the only genuine answer to "bcrypt pegs a CPU".

What it costs is resident memory on the serving side: 19 MiB allocated per derivation in flight, against bcrypt's 5 KiB. A hundred concurrent logins is 1.9 GiB, which is a limit to set rather than a cost to absorb.

**The `threads` option trades the rate for the latency.** More lanes spread one derivation over cores, so `password_hash($p, PASSWORD_ARGON2ID, array("threads" => 2))` lowers what one login waits. It does not raise what the host serves: the cores a lane takes are cores the next login was going to run on. Set it when a single login's latency is the complaint and leave it at 1 when the rate is.

## Choosing a password algorithm

| If                                                     | Store with                                                   |
|--------------------------------------------------------|--------------------------------------------------------------|
| Hashes have to be readable by php, or by anything else | `PASSWORD_DEFAULT`, which is bcrypt, as php's is             |
| Login latency matters and memory is available          | `PASSWORD_ARGON2ID` at m=19456, t=2, p=1                     |
| Memory per request is tight                            | `PASSWORD_ARGON2ID` at m=9216, t=4, p=1                      |
| bcrypt has to stay and cost 12 is too slow             | `PASSWORD_BCRYPT` at cost 10, which is the lowest defensible |
| The secret is a token rather than a password           | `hash("sha256", ...)` and `hash_equals`, not this family     |

```php
// 29.65 ms, 19 MiB, and a guess costs an attacker the same 19 MiB.
$opts   = array("memory_cost" => 19456, "time_cost" => 2, "threads" => 1);
$stored = password_hash($password, PASSWORD_ARGON2ID, $opts);

// The upgrade path off bcrypt, taken on a successful login.
if (password_verify($submitted, $stored) && password_needs_rehash($stored, PASSWORD_ARGON2ID, $opts)) {
    $stored = password_hash($submitted, PASSWORD_ARGON2ID, $opts);
}
```

`PASSWORD_DEFAULT` stays bcrypt, because php's does and a hash is a stored value two runtimes have to agree on. Both argon2 variants are registered and encode to the PHC string form libargon2 writes, so a hash written here verifies under php and the other way around; [password_argon2.phpt](../../../tests/fixtures/stdlib/password_argon2.phpt) carries pasted php output for both directions. `argon2i` exists because php registers it; `argon2id` is the one to choose, and RFC 9106 says so.

`php -r` is not a benchmark for this. Measure on the hardware that will serve, with the fixture or the Go benchmark above, and set the parameters from that rather than from this table: a derivation whose cost was chosen on a developer laptop is the wrong cost on everything else.

**A hash that cannot match costs nothing.** `password_verify` reads the algorithm and the work factors out of the stored value, so a value that is neither argon2 nor long enough to be bcrypt is `false` without a derivation: 4.16 ns for `""`, 9.14 ns for a truncated bcrypt hash, no allocation. php answers the empty case in 31 ns.

`password_verify($password, "")` is the spelling an application uses when a user lookup found nothing, and it is deliberately not a derivation. Making a missing account take as long as a wrong password is a rate-limiting problem, not a hashing one: the fix is to limit attempts per address and per account, which bounds what an attacker learns from any timing at all. A fixed-cost decoy does not, because its cost is fixed at one algorithm and one parameter set while the real path's is whatever the stored hash says.

## Implementation

`md5`, `sha1` and the `hash` family are in [stdlib/crypto/hash.go](../../../stdlib/crypto/hash.go); `crc32` is in [stdlib/core/strings.go](../../../stdlib/core/strings.go) with the rest of the string functions, because it returns an integer and is reached for as one. The password functions are in [stdlib/crypto/password.go](../../../stdlib/crypto/password.go) and the CSPRNG in [stdlib/crypto/random.go](../../../stdlib/crypto/random.go).

The argon2 encoding and its parsing are in [stdlib/crypto/argon2.go](../../../stdlib/crypto/argon2.go), separate from the registrations because the PHC string form is the whole of the interop and is the part a reader checks against php.

Four fixtures cover the surface: [hashing.phpt](../../../tests/fixtures/stdlib/hashing.phpt) for width and collisions, [hash.phpt](../../../tests/fixtures/stdlib/hash.phpt) for the vectors of every registered algorithm and for `hash_hmac` and `hash_equals`, [password_hash.phpt](../../../tests/fixtures/stdlib/password_hash.phpt) for bcrypt, and [password_argon2.phpt](../../../tests/fixtures/stdlib/password_argon2.phpt) for the two argon2 variants.
