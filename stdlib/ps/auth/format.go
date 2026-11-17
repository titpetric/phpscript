package auth

import (
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/stdlib/ps/crypto"
)

// shaPrefix is the {SHA} form's prefix, ahead of a base64-encoded sha1.
const shaPrefix = "{SHA}"

// verify reports whether secret produced stored, answering false for an unknown format.
func verify(secret, stored string) bool {
	switch {
	case isSHA(stored):
		return shaVerify(secret, stored)
	case crypto.IsAPR1(stored):
		return crypto.APR1Verify(secret, stored)
	default:
		return crypto.PasswordVerify(secret, stored)
	}
}

// formatError reports a stored value this build refuses, naming what to write instead.
func formatError(stored string) error {
	if strings.HasPrefix(stored, "$1$") {
		return fmt.Errorf("md5-crypt is php's $1$ and not apache's $apr1$; rewrite the line with htpasswd -B")
	}
	if stored == "" {
		return fmt.Errorf("no password hash")
	}
	// Plain text is the dangerous one, and nothing detects it: a line apache
	// would read as a plain password is any line the formats above leave
	// unclaimed, and a truncated hash looks the same. Refusing the whole
	// class stops a corrupt file from authenticating.
	return fmt.Errorf("unreadable password hash, want bcrypt ($2y$), argon2, apache md5 ($apr1$) or {SHA}; write it with htpasswd -B")
}

// readable reports whether a stored value is a format verify can decide.
func readable(stored string) bool {
	return isSHA(stored) || crypto.IsAPR1(stored) || crypto.IsPasswordHash(stored)
}

// isSHA reports whether stored is the {SHA} form, its base64 digest included.
func isSHA(stored string) bool {
	encoded, ok := strings.CutPrefix(stored, shaPrefix)
	if !ok {
		return false
	}
	sum, err := base64.StdEncoding.DecodeString(encoded)
	return err == nil && len(sum) == sha1.Size
}

// shaVerify compares one sha1 of secret against the stored digest in constant time.
func shaVerify(secret, stored string) bool {
	want, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, shaPrefix))
	if err != nil {
		return false
	}
	sum := sha1.Sum([]byte(secret))
	return subtle.ConstantTimeCompare(sum[:], want) == 1
}
