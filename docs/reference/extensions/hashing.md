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

`hash_algos()` answers the seven names this build carries: `md5`, `sha1`, `sha224`, `sha256`, `sha384`, `sha512` and `crc32b`. The list is a seventh of php's, so a script that lets a user pick an algorithm reads the list. `password_algos()` answers the three php answers, in php's order: `2y`, `argon2i` and `argon2id`.

This page covers what each function's output is worth and which question each one answers. The wider security posture they sit inside - secrets, key handling, what a vhost is allowed to reach - is issue #139's remaining scope and is documented in [security.md](../../security.md).

## Checksums, digests, keyed digests and password hashes

These four make four different claims. A function that makes one of them guarantees nothing about the other three.

**A checksum** detects accidental corruption. `crc32` is one: 32 bits, a few cycles per byte, and no claim against someone who wants two inputs to agree. CRC-32 is linear over GF(2), so an attacker who can append four bytes to a message sets its checksum to any value they like, in constant time and with no search. Use it for a transport check, a cache bucket or an ETag over content you produced yourself. An attacker who chooses the input defeats it.

**A message digest** claims collision resistance: that nobody can produce two inputs with the same output. md5 and sha1 fail that claim, against published attacks; `sha256` upward hold it.

**A keyed digest** claims authenticity: that nobody without the key can produce a valid tag. `hash_hmac` is one. A signed cookie, a webhook signature and a bearer-token tag are built from it, at a cost a request pays per call; [What a derivation costs](#what-a-derivation-costs) prices it against the alternatives.

**A password hash** claims that the stored value is expensive to invert even though the input has little entropy. `password_hash` is the only function here that claims it. [Why a password needs a cost parameter](#why-a-password-needs-a-cost-parameter) states why no digest substitutes.

## What width buys

A generic collision search is a birthday search, so it finds a pair after about `2**(bits/2)` inputs. The output width is the only input to that arithmetic:

| Algorithm | Bits | Hex characters | Generic collision search |
|-----------|-----:|---------------:|-------------------------:|
| `crc32b`  |   32 |              8 |                  `2**16` |
| `md5`     |  128 |             32 |                  `2**64` |
| `sha1`    |  160 |             40 |                  `2**80` |
| `sha224`  |  224 |             56 |                 `2**112` |
| `sha256`  |  256 |             64 |                 `2**128` |
| `sha384`  |  384 |             96 |                 `2**192` |
| `sha512`  |  512 |            128 |                 `2**256` |

[tests/fixtures/stdlib/hashing.phpt](../../../tests/fixtures/stdlib/hashing.phpt) derives that table from the functions themselves and then exhibits the left-hand end of it: a `crc32` collision found by a birthday search over 60411 random inputs, against the `2**16` the row predicts. The same fixture truncates md5, sha1 and sha256 to 32 bits and collides each of them in the same number of tries. Truncating `sha256` to 32 bits reduces its collision search to `crc32`'s.

`sha256` reaches `2**128`, the current floor for a digest that has to resist collisions. Wider digests cost throughput for a margin nothing uses.

## Published collisions against md5 and sha1

Published attacks place both far below their birthday bound:

| Algorithm | Attack                     | Published                                     | Work    |
|-----------|----------------------------|-----------------------------------------------|--------:|
| md5       | Identical-prefix collision | Wang and Yu, EUROCRYPT 2005                   | `2**39` |
| md5       | Identical-prefix collision | Klima 2006, Stevens 2006                      | `2**24` |
| md5       | Chosen-prefix collision    | Stevens, Sotirov et al., rogue CA, 2008       | `2**50` |
| sha1      | Identical-prefix collision | SHAttered, Stevens et al., 2017               | `2**63` |
| sha1      | Chosen-prefix collision    | SHA-1 is a Shambles, Leurent and Peyrin, 2020 | `2**63` |

An md5 collision is seconds of laptop time, and the fixture carries the Wang-Yu pair as two 128-byte messages with one md5 between them. The sha1 chosen-prefix attack cost about 45,000 US dollars of rented GPU in 2020. That cost removed sha1 from certificates and from PGP key identity.

Preimage resistance of both is unbroken: nobody can produce an input for a given md5. A stored password is attacked by guessing, which preimage resistance does nothing against.

md5 and sha1 remain sound where no adversary chooses the input: a cache key, a change detector over your own files, a shard selector, an ETag. They fail wherever an attacker can influence a value and benefit from duplicating it: a content address, a deduplication key over uploads, a commitment, a signature input, a password.

`crc32` carries no cryptographic claim to weaken. It is an error detector.

## crc16 and crc64 do not exist here, or in php

Neither name is in php's `hash_algos()`, so there is no behaviour to be compatible with and neither is registered. A script wanting a short checksum takes `crc32`. A script wanting a 64-bit one truncates a real digest: `substr(hash("sha256", $data), 0, 16)`. The truncated forms of SHA-2 are defined exactly that way, and 64 bits of output is a `2**32` collision search, too narrow for anything adversarial.

## What this build does not carry

php's `hash_algos()` has sixty entries. The absent ones, and what each is for:

| Absent                                   | What it would be for                                                                                                                                       |
|------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `sha3-256`, `sha3-512`                   | A sponge construction with no length-extension property. Go has `crypto/sha3`.                                                                             |
| `sha512/256`                             | SHA-2 on 64-bit words truncated to 256 bits: faster than `sha256` on a 64-bit CPU, and length extension does not apply. Go has `crypto/sha512.Sum512_256`. |
| `xxh3`, `xxh64`                          | A fast non-cryptographic digest, for a cache key.                                                                                                          |
| `hash_pbkdf2`                            | A key derived from a password or a shared secret, with an iteration count.                                                                                 |
| `hash_init`, `hash_update`, `hash_final` | Digesting a stream without holding it in memory.                                                                                                           |
| `hash_file`, `hash_hmac_file`            | The same over a path.                                                                                                                                      |

Length extension is the one of these a script can hit by accident. `hash("sha256", $secret . $message)` is forgeable by anyone who knows the length of `$secret`: SHA-2 lets them continue the digest without the secret. `hash_hmac("sha256", $message, $secret)` resists it, and it is registered.

## Why a password needs a cost parameter

A digest is fast. An attacker with a stolen table guesses: they run the same function over a wordlist and compare. A salt stops one precomputed table from answering every row at once, and leaves the guessing rate unchanged. An attacker running an unkeyed digest spends per guess what the server spends per login, so the ratio between the two is 1 to 1 at any width. The `sha256` row in [What a derivation costs](#what-a-derivation-costs) is that figure.

No bit width changes the ratio. `sha512` and `sha256` cost an attacker the same per guess, and neither has a parameter that raises it. At equal login latency a salted digest is a weaker store than `password_hash`.

A password hash carries a **cost parameter**: a number multiplying the work per guess, set so that one derivation is affordable for the one login a user performs and unaffordable for the billions an attacker needs. `PASSWORD_BCRYPT_DEFAULT_COST` is its default here:

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

**The fast digests belong on the token.** A session identifier or a bearer token is 128 bits from `random_bytes`, so it holds nothing to guess and nothing for a cost parameter to defend. Store a digest of it and compare with `hash_equals`. An authenticated request then costs one digest:

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

A deployment whose login latency is a problem has three levers, in this order: issue a token, which moves the derivation from every request to one per session; tune the algorithm and its parameters against a measurement on the hardware that will run it; and bound how many derivations can be in flight at once, because each one holds a core for its whole duration.

## What a derivation costs

All numbers from [stdlib/crypto/password_test.go](../../../stdlib/crypto/password_test.go) on an Intel N150, Go 1.27, `CGO_ENABLED=0`, `GOMEMLIMIT=2GiB`, one pinned core: `benchstat` medians over `-count 6` at `-benchtime=100ms`. `password_verify` runs the same derivation at the parameters the hash records, so one column covers both functions.

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

The `sec/op` column is what one login costs and what one guess costs an attacker, at the same parameters. The `B/op` column is what each side holds while doing it.

The cost parameter is a power of two, so each bcrypt step doubles both columns of work. Lowering it is the only bcrypt lever, and it halves what an attacker pays as well.

### How many logins that is

A bcrypt derivation holds one core and a small fixed working set, so concurrent ones scale with the core count. An argon2id derivation holds its whole `memory_cost` and streams it, so concurrent ones contend for memory bandwidth and scale below the core count. `BenchmarkPasswordVerifyThroughput` measures the rate under `-cpu 1,2,4`: `sec/op` there is wall clock per completed login across the cores in play, and logins per second is its reciprocal.

| Stored at               | 1 core   | 2 cores  | 4 cores  | Scaling | Logins/sec | Per hour | Per day |
|-------------------------|---------:|---------:|---------:|--------:|-----------:|---------:|--------:|
| `bcrypt` cost 12        | 233.2 ms | 137.4 ms | 69.43 ms |    3.4x |       14.4 |    51.8K |   1.24M |
| `argon2id` m=19456, t=2 | 29.16 ms | 17.70 ms | 13.04 ms |    2.2x |       76.7 |     276K |   6.63M |

Scaling is the one-core figure over the four-core figure. argon2id falls short of the core count because its working set contends for bandwidth.

**argon2id adds a memory cost.** bcrypt's working set fits in a GPU core's local memory, so one card runs thousands of guesses in parallel. argon2id's does not fit, and the parallelism an attacker buys is bounded by memory bandwidth. Both sides pay the `B/op` column: the attacker per guess, and the server once per derivation in flight. Multiply it by the concurrency limit, and set that limit.

**A token costs the `hash_hmac` row; a password costs its algorithm's row.** A request that re-derives a password hash pays the whole derivation, and no choice of algorithm recovers the difference.

**The `threads` option trades the host's rate for one login's latency.** More lanes spread a single derivation across cores, so `password_hash($p, PASSWORD_ARGON2ID, array("threads" => 2))` lowers what one login waits. The host's rate is unchanged: a lane takes a core the next login would have used. Raise it when one login's latency is the complaint, and leave it at 1 for throughput.

## Choosing a password algorithm

| If                                                     | Store with                                                   |
|--------------------------------------------------------|--------------------------------------------------------------|
| Hashes have to be readable by php, or by anything else | `PASSWORD_DEFAULT`, which is bcrypt, as php's is             |
| Login latency matters and memory is available          | `PASSWORD_ARGON2ID` at m=19456, t=2, p=1                     |
| Memory per request is tight                            | `PASSWORD_ARGON2ID` at m=9216, t=4, p=1                      |
| bcrypt has to stay and cost 12 is too slow             | `PASSWORD_BCRYPT` at cost 10, which is the lowest defensible |
| The secret is a high-entropy token                     | `hash("sha256", ...)` and `hash_equals`, not this family     |

```php
// 29.65 ms, 19 MiB, and a guess costs an attacker the same 19 MiB.
$opts   = array("memory_cost" => 19456, "time_cost" => 2, "threads" => 1);
$stored = password_hash($password, PASSWORD_ARGON2ID, $opts);

// The upgrade path off bcrypt, taken on a successful login.
if (password_verify($submitted, $stored) && password_needs_rehash($stored, PASSWORD_ARGON2ID, $opts)) {
    $stored = password_hash($submitted, PASSWORD_ARGON2ID, $opts);
}
```

`PASSWORD_DEFAULT` stays bcrypt, because php's does and a hash is a stored value two runtimes have to agree on. Both argon2 variants are registered and encode to the PHC string form libargon2 writes, so a hash written here verifies under php and the other way around; [password_argon2.phpt](../../../tests/fixtures/stdlib/password_argon2.phpt) carries pasted php output for both directions. `argon2i` is registered because php registers it. RFC 9106 recommends `argon2id`.

Set the parameters from a measurement on the hardware that will serve, taken with the fixture or the Go benchmark above. A cost chosen on a developer laptop is the wrong cost everywhere else, and `php -r` measures the laptop.

### A hash that cannot match costs nothing

`password_verify` reads the algorithm and the work factors out of the stored value, so a value that is neither argon2 nor long enough to be bcrypt is `false` with no derivation. `BenchmarkPasswordVerifyRefuse`, same conditions as the cost table:

| Stored value            | sec/op  | allocs/op |
|-------------------------|--------:|----------:|
| `""`                    | 4.16 ns |         0 |
| a non-hash string       | 9.28 ns |         0 |
| a truncated bcrypt hash | 9.14 ns |         0 |
| an argon2 1.0 hash      |  341 ns |         4 |
| php 8.5.4, `""`         |   31 ns |       n/a |

`password_verify($password, "")` is the spelling an application uses when a user lookup found nothing, and it spends no derivation. A missing account and a wrong password therefore answer in different times. Closing that gap belongs to rate limiting: limit attempts per address and per account, which bounds what an attacker learns from any timing at all. A fixed-cost decoy bounds nothing, because its cost sits at one algorithm and one parameter set while the real path's is whatever the stored hash records.

## Implementation

`md5`, `sha1` and the `hash` family are in [stdlib/crypto/hash.go](../../../stdlib/crypto/hash.go); `crc32` is in [stdlib/core/strings.go](../../../stdlib/core/strings.go) with the rest of the string functions, because it returns an integer and is reached for as one. The password functions are in [stdlib/crypto/password.go](../../../stdlib/crypto/password.go) and the CSPRNG in [stdlib/crypto/random.go](../../../stdlib/crypto/random.go).

The argon2 encoding and its parsing are in [stdlib/crypto/argon2.go](../../../stdlib/crypto/argon2.go), separate from the registrations because the PHC string form is the whole of the interop and is the part a reader checks against php.

Four fixtures cover the surface: [hashing.phpt](../../../tests/fixtures/stdlib/hashing.phpt) for width and collisions, [hash.phpt](../../../tests/fixtures/stdlib/hash.phpt) for the vectors of every registered algorithm and for `hash_hmac` and `hash_equals`, [password_hash.phpt](../../../tests/fixtures/stdlib/password_hash.phpt) for bcrypt, and [password_argon2.phpt](../../../tests/fixtures/stdlib/password_argon2.phpt) for the two argon2 variants.
