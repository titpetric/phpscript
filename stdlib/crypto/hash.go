package crypto

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"

	"github.com/titpetric/phpscript/runner"
)

// RegisterHash installs PHP's message-digest pair. Like the CSPRNG, a digest
// is something a script cannot write for itself at any usable speed, which is
// what qualifies it for this package; the algorithm-agnostic hash() family is
// deliberately absent until something needs more than these two names.
//
// Both return string, not []byte: a PHP string is a Go string in this
// runtime, and a []byte would reach scripts as a foreign Go value that
// strlen and concatenation do not recognise.
func RegisterHash(rt *runner.Runtime) {
	// md5 returns the MD5 hash of $string as 32 lowercase hex characters, or as 16 raw bytes when $binary is true.
	rt.RegisterFunc("md5", func(str string, binary ...bool) string {
		sum := md5.Sum([]byte(str))
		if len(binary) > 0 && binary[0] {
			return string(sum[:])
		}
		return hex.EncodeToString(sum[:])
	})

	// sha1 returns the SHA-1 hash of $string as 40 lowercase hex characters, or as 20 raw bytes when $binary is true.
	rt.RegisterFunc("sha1", func(str string, binary ...bool) string {
		sum := sha1.Sum([]byte(str))
		if len(binary) > 0 && binary[0] {
			return string(sum[:])
		}
		return hex.EncodeToString(sum[:])
	})
}
