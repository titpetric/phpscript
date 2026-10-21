package crypto

import (
	"crypto/md5"
	"crypto/subtle"
	"strings"
)

// apr1Magic is the identifier Apache's md5-crypt variant writes, and the whole
// of what separates it from the `$1$` md5-crypt php's crypt() reads: the magic
// string goes into the first digest, so the same password and salt produce a
// different hash under each. php verifies no `$apr1$` line - crypt() answers
// "*0" for the setting - so this derivation lives here and its test vectors
// come from `openssl passwd -apr1`.
const apr1Magic = "$apr1$"

// apr1Rounds is the iteration count the algorithm fixes. No parameter carries
// it: nothing in the encoded form records it and no caller can raise it, so a
// guess costs 1000 md5 compressions for every file. That is tens of
// microseconds, a hundred thousand times under bcrypt at the default cost, and
// the reason docs/security.md names `htpasswd -B`.
const apr1Rounds = 1000

// apr1SaltMax is the salt length Apache generates and reads. apr_md5_encode
// truncates a longer field, so this one does too.
const apr1SaltMax = 8

// apr1Alphabet is crypt(3)'s base64 ordering, which is neither standard base64
// nor the PHC form argon2 writes.
const apr1Alphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// APR1Verify reports whether password produced the `$apr1$salt$hash` in stored.
// A value that is not one returns false, as PasswordVerify does for a hash it
// cannot read.
func APR1Verify(password, stored string) bool {
	salt, _, ok := apr1Split(stored)
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(apr1Encode(password, salt)), []byte(stored)) == 1
}

// IsAPR1 reports whether a stored value claims to be this format, which a
// password file carrying a format per line asks.
func IsAPR1(stored string) bool {
	_, _, ok := apr1Split(stored)
	return ok
}

// apr1Split reads the salt and the tag out of the encoded form. The tag comes
// back for the shape check alone: a verify recomputes the whole string, so
// nothing compares against it directly.
func apr1Split(stored string) (salt, tag string, ok bool) {
	rest, found := strings.CutPrefix(stored, apr1Magic)
	if !found {
		return "", "", false
	}
	salt, tag, found = strings.Cut(rest, "$")
	if !found || len(salt) > apr1SaltMax || tag == "" {
		return "", "", false
	}
	return salt, tag, true
}

// apr1Encode is apr_md5_encode: the derivation Apache's htpasswd writes with no
// flag, transcribed from APR's own implementation.
//
// Change any step and the hash stops matching what htpasswd wrote, including
// the two steps that read as mistakes. The password length drives a loop that
// mixes either a zero byte or the password's first byte, so two passwords
// differing only in length take different paths; and the 1000 rounds alternate
// what they digest on the index, so the derivation is serial.
func apr1Encode(password, salt string) string {
	if len(salt) > apr1SaltMax {
		salt = salt[:apr1SaltMax]
	}

	// The first digest commits to the password, the magic and the salt, then
	// takes as many bytes of a second digest as the password is long.
	ctx := md5.New()
	ctx.Write([]byte(password))
	ctx.Write([]byte(apr1Magic))
	ctx.Write([]byte(salt))

	inner := md5.New()
	inner.Write([]byte(password))
	inner.Write([]byte(salt))
	inner.Write([]byte(password))
	final := inner.Sum(nil)

	for length := len(password); length > 0; length -= md5.Size {
		ctx.Write(final[:min(length, md5.Size)])
	}

	// APR zeroes its digest buffer here and then writes one byte from it per
	// set bit of the password length. The zero byte is the "mixed with a null"
	// case, and a hash written by htpasswd carries it.
	zero := []byte{0}
	first := []byte(password)
	for length := len(password); length != 0; length >>= 1 {
		if length&1 == 1 {
			ctx.Write(zero)
			continue
		}
		ctx.Write(first[:1])
	}
	final = ctx.Sum(nil)

	// The rounds are the whole cost. Each one digests the password and the
	// previous output in an order the index decides, so a round waits on the
	// one before it.
	for i := 0; i < apr1Rounds; i++ {
		round := md5.New()
		if i&1 == 1 {
			round.Write([]byte(password))
		} else {
			round.Write(final)
		}
		if i%3 != 0 {
			round.Write([]byte(salt))
		}
		if i%7 != 0 {
			round.Write([]byte(password))
		}
		if i&1 == 1 {
			round.Write(final)
		} else {
			round.Write([]byte(password))
		}
		final = round.Sum(nil)
	}

	var out strings.Builder
	out.WriteString(apr1Magic)
	out.WriteString(salt)
	out.WriteString("$")
	// The byte order is APR's and not the digest's. Five groups of three bytes
	// become four characters each, the last group reaching back to byte 5, and
	// byte 11 goes out on its own as two characters.
	for _, group := range [5][3]int{{0, 6, 12}, {1, 7, 13}, {2, 8, 14}, {3, 9, 15}, {4, 10, 5}} {
		value := uint32(final[group[0]])<<16 | uint32(final[group[1]])<<8 | uint32(final[group[2]])
		apr1Base64(&out, value, 4)
	}
	apr1Base64(&out, uint32(final[11]), 2)
	return out.String()
}

// apr1Base64 is crypt(3)'s to64: n characters of value, least significant six
// bits first, which is the reverse of what base64 writes.
func apr1Base64(out *strings.Builder, value uint32, n int) {
	for ; n > 0; n-- {
		out.WriteByte(apr1Alphabet[value&0x3f])
		value >>= 6
	}
}
