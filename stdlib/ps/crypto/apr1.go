package crypto

import (
	"crypto/md5"
	"crypto/subtle"
	"strings"
)

// apr1Magic is the identifier Apache's md5-crypt variant writes in place of $1$.
const apr1Magic = "$apr1$"

// apr1Rounds is the algorithm's fixed 1000 md5 iterations, that no parameter raises.
const apr1Rounds = 1000

// apr1SaltMax is the salt length Apache generates, truncating a longer field.
const apr1SaltMax = 8

// apr1Alphabet is crypt(3)'s base64 ordering, unlike standard base64.
const apr1Alphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// APR1Verify reports whether password produced the `$apr1$salt$hash` in stored.
// A different value returns false, as PasswordVerify does.
func APR1Verify(password, stored string) bool {
	salt, _, ok := apr1Split(stored)
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(apr1Encode(password, salt)), []byte(stored)) == 1
}

// IsAPR1 reports whether the stored value claims the `$apr1$` format.
func IsAPR1(stored string) bool {
	_, _, ok := apr1Split(stored)
	return ok
}

// apr1Split reads the salt and tag, the tag checked for shape.
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

// apr1Encode is apr_md5_encode, transcribed from APR and matching every step htpasswd writes.
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
	// case that htpasswd writes.
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

	// The rounds are the whole cost. Each round digests the password and the
	// previous output in an order the index decides; a round waits on the round
	// before it.
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
	// byte 11 goes out alone as two characters.
	for _, group := range [5][3]int{{0, 6, 12}, {1, 7, 13}, {2, 8, 14}, {3, 9, 15}, {4, 10, 5}} {
		value := uint32(final[group[0]])<<16 | uint32(final[group[1]])<<8 | uint32(final[group[2]])
		apr1Base64(&out, value, 4)
	}
	apr1Base64(&out, uint32(final[11]), 2)
	return out.String()
}

// apr1Base64 writes n characters of value least significant six bits first.
func apr1Base64(out *strings.Builder, value uint32, n int) {
	for ; n > 0; n-- {
		out.WriteByte(apr1Alphabet[value&0x3f])
		value >>= 6
	}
}
