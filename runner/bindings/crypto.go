package bindings

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

// init contributes the algorithm-agnostic digests to stdlib.Register.
func init() {
	runner.RegisterBinding(registerCrypto)
}

// digests is the algorithm table, spelled the way PHP spells the names. It is
// deliberately shorter than PHP's hash_algos(): every entry here is one a Go
// binary already carries, and hash_algos() below answers this table rather than
// PHP's, so a script can ask instead of guessing.
//
// stdlib/crypto/hash.go binds md5() and sha1() and says the agnostic family
// stays absent "until something needs more than these two names". Storing a
// token as a digest that can still be looked up by index is that need: bcrypt
// cannot answer a lookup, and a fast digest chosen by name can.
var digests = map[string]func() hash.Hash{
	"md5":    md5.New,
	"sha1":   sha1.New,
	"sha224": sha256.New224,
	"sha256": sha256.New,
	"sha384": sha512.New384,
	"sha512": sha512.New,
	// crc32b is PHP's name for the ordinary IEEE CRC-32, and it is here because
	// it is what a script reaching for a cheap non-cryptographic checksum
	// types. crc32() the function is a different spelling of the same thing and
	// is bound elsewhere.
	"crc32b": func() hash.Hash { return crc32.NewIEEE() },
}

func registerCrypto(rt *runner.Runtime) {
	// hash returns the $algo digest of $data as lowercase hex characters, or as raw bytes when $binary is true; $algo is one of the names hash_algos() answers, which is a subset of PHP's.
	rt.RegisterFunc("hash", func(algo, data string, binary ...bool) (string, error) {
		h, err := newDigest("hash", algo)
		if err != nil {
			return "", err
		}
		h.Write([]byte(data))
		return digestString(h.Sum(nil), binary), nil
	})

	// hash_hmac returns the $algo keyed digest of $data under $key as lowercase hex characters, or as raw bytes when $binary is true; a key longer than the algorithm's block size is itself digested first, which is HMAC's own rule and not a choice made here.
	rt.RegisterFunc("hash_hmac", func(algo, data, key string, binary ...bool) (string, error) {
		factory, ok := digests[algo]
		if !ok {
			return "", unknownAlgo("hash_hmac", algo)
		}
		mac := hmac.New(factory, []byte(key))
		mac.Write([]byte(data))
		return digestString(mac.Sum(nil), binary), nil
	})

	// hash_equals reports whether $known_string and $user_string are the same, in time that does not depend on how far along they first differ; strings of different lengths are never equal, and the comparison of a wrong-length guess is the one case that does leak, because the lengths are compared first.
	rt.RegisterFunc("hash_equals", func(known_string, user_string string) bool {
		if len(known_string) != len(user_string) {
			return false
		}
		return subtle.ConstantTimeCompare([]byte(known_string), []byte(user_string)) == 1
	})

	// hash_algos returns the algorithm names hash() and hash_hmac() accept here, sorted; the list is shorter than PHP's, so a script that offers a choice should read it rather than assume one.
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

// newDigest resolves an algorithm name to a fresh digest.
func newDigest(fn, algo string) (hash.Hash, error) {
	factory, ok := digests[algo]
	if !ok {
		return nil, unknownAlgo(fn, algo)
	}
	return factory(), nil
}

// unknownAlgo phrases the refusal the way PHP 8 phrases its ValueError, so a
// script that catches it reads a familiar message. There is nothing to clamp to
// here: a digest under a name this build does not carry has no nearest answer.
func unknownAlgo(fn, algo string) error {
	return fmt.Errorf("%s(): Argument #1 ($algo) must be a valid hashing algorithm, %q given", fn, algo)
}

// digestString renders a sum the way PHP does: hex unless the call asked for
// the raw bytes, which reach a script as an ordinary string because that is
// what a PHP string is in this runtime.
func digestString(sum []byte, binary []bool) string {
	if len(binary) > 0 && binary[0] {
		return string(sum)
	}
	return hex.EncodeToString(sum)
}
