package crypto

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"hash"
	"hash/crc32"
	"sort"

	"github.com/titpetric/phpscript/runner"
)

// RegisterHash installs PHP's message digests: md5() and sha1(), and the
// algorithm-agnostic hash() family over the table below. Like the CSPRNG, a
// digest is something a script cannot write for itself at any usable speed.
// That qualifies it for this package.
//
// Both return string, not []byte: a PHP string is a Go string in this
// runtime, and a []byte would reach scripts as a foreign Go value that
// strlen and concatenation do not recognise.
func RegisterHash(rt *runner.Runtime) {
	// md5 returns the MD5 hash of $string as 32 lowercase hex characters, or
	// as 16 raw bytes when $binary is true.
	rt.RegisterFunc("md5", func(str string, binary ...bool) string {
		sum := md5.Sum([]byte(str))
		if len(binary) > 0 && binary[0] {
			return string(sum[:])
		}
		return hex.EncodeToString(sum[:])
	})

	// sha1 returns the SHA-1 hash of $string as 40 lowercase hex characters,
	// or as 20 raw bytes when $binary is true.
	rt.RegisterFunc("sha1", func(str string, binary ...bool) string {
		sum := sha1.Sum([]byte(str))
		if len(binary) > 0 && binary[0] {
			return string(sum[:])
		}
		return hex.EncodeToString(sum[:])
	})

	// hash returns the $algo digest of $data as lowercase hex characters, or
	// as raw bytes when $binary is true; $algo is one of the names
	// hash_algos() answers, which is a subset of PHP's.
	rt.RegisterFunc("hash", func(algo, data string, binary ...bool) (string, error) {
		h, err := newDigest("hash", algo)
		if err != nil {
			return "", err
		}
		h.Write([]byte(data))
		return digestString(h.Sum(nil), binary), nil
	})

	// hash_hmac returns the $algo keyed digest of $data under $key as
	// lowercase hex characters, or as raw bytes when $binary is true; a key
	// longer than the algorithm's block size is itself digested first, which
	// is HMAC's own rule and not a choice made here.
	rt.RegisterFunc("hash_hmac", func(algo, data, key string, binary ...bool) (string, error) {
		factory, ok := digests[algo]
		if !ok {
			return "", unknownAlgo("hash_hmac", algo)
		}
		mac := hmac.New(factory, []byte(key))
		mac.Write([]byte(data))
		return digestString(mac.Sum(nil), binary), nil
	})

	// hash_equals reports whether $known_string and $user_string are the
	// same, in time that does not depend on how far along they first differ;
	// strings of different lengths are never equal, and the comparison of a
	// wrong-length guess is the one case that does leak, because the length
	// check runs first.
	rt.RegisterFunc("hash_equals", func(known_string, user_string string) bool {
		if len(known_string) != len(user_string) {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(known_string), []byte(user_string)) == 1
	})

	// hash_algos returns the algorithm names hash() and hash_hmac() accept
	// here, sorted. The list is a seventh of PHP's, and a script that lets a user
	// pick an algorithm reads the list.
	rt.RegisterFunc("hash_algos", func() []any {
		names := make([]string, 0, len(digests))
		for name := range digests {
			names = append(names, name)
		}
		sort.Strings(names)

		out := make([]any, 0, len(names))
		for _, name := range names {
			out = append(out, name)
		}
		return out
	})
}

// digests maps PHP's algorithm names to the digest factories this build holds.
var digests = map[string]func() hash.Hash{
	"md5":    md5.New,
	"sha1":   sha1.New,
	"sha224": sha256.New224,
	"sha256": sha256.New,
	"sha384": sha512.New384,
	"sha512": sha512.New,
	// crc32b is PHP's name for the ordinary IEEE CRC-32, the name a script
	// reaching for a cheap non-cryptographic checksum types. crc32() the
	// function spells the same thing and stdlib/ps/core registers it.
	"crc32b": func() hash.Hash { return crc32.NewIEEE() },
}

// newDigest resolves an algorithm name to a fresh digest.
func newDigest(fn, algo string) (hash.Hash, error) {
	factory, ok := digests[algo]
	if !ok {
		return nil, unknownAlgo(fn, algo)
	}
	return factory(), nil
}

// unknownAlgo returns PHP 8's ValueError text for a digest name this build does not hold.
func unknownAlgo(fn, algo string) error {
	return fmt.Errorf("%s(): Argument #1 ($algo) must be a valid hashing algorithm, %q given", fn, algo)
}

// digestString renders a sum as hex, or raw bytes when the call passes binary true.
func digestString(sum []byte, binary []bool) string {
	if len(binary) > 0 && binary[0] {
		return string(sum)
	}
	return hex.EncodeToString(sum)
}
