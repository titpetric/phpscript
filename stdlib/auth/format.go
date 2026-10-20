package auth

import (
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/stdlib/crypto"
)

// shaPrefix is what `htpasswd -s` writes: the literal {SHA} and the base64 of
// one sha1 of the password. No salt and no cost parameter, so a stolen file
// costs a wordlist lookup per row. This build reads it because deployments
// carry such files; docs/security.md prices it against the rest.
const shaPrefix = "{SHA}"

// verify reports whether secret produced stored, dispatching on the shape of
// the stored value the way password_verify does.
//
// A value no format claims answers false, with no error. A file this package
// parsed carries none, because the parse refuses such a line where it sits;
// the fallthrough keeps a store built another way from authenticating anyone.
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

// formatError reports a stored value this build will not read, naming what to
// write it with instead. The parse refuses such a file whole, so an operator
// acts on this message.
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

// readable reports whether a stored value is a format verify can decide. The
// parse runs it per line, so the constructor refuses a bad file and no login
// discovers one.
func readable(stored string) bool {
	return isSHA(stored) || crypto.IsAPR1(stored) || crypto.IsPasswordHash(stored)
}

// isSHA reports whether a stored value is the {SHA} form, base64 included: the
// prefix alone would claim a truncated line that nothing can verify.
func isSHA(stored string) bool {
	encoded, ok := strings.CutPrefix(stored, shaPrefix)
	if !ok {
		return false
	}
	sum, err := base64.StdEncoding.DecodeString(encoded)
	return err == nil && len(sum) == sha1.Size
}

// shaVerify compares one sha1 of the secret against the stored digest. The
// comparison is constant time for the form's sake; the derivation it protects
// is one digest, so an attacker does not need the comparison.
func shaVerify(secret, stored string) bool {
	want, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, shaPrefix))
	if err != nil {
		return false
	}
	sum := sha1.Sum([]byte(secret))
	return subtle.ConstantTimeCompare(sum[:], want) == 1
}
