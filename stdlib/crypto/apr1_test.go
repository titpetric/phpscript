package crypto

import (
	"testing"
)

// apr1Vectors are openssl's output, the reference for this derivation because
// php has none: `crypt()` answers "*0" for an `$apr1$` setting. Each row comes
// from
//
//	openssl passwd -apr1 -salt <salt> <password>
//
// so a change here that still passes is a change openssl disagrees with.
var apr1Vectors = []struct {
	name     string
	password string
	stored   string
}{
	{"eight character salt", "hunter2hunter2", "$apr1$Zx1kLm9q$BwLItAYS8lPw/bCsIXHax1"},
	{"a password with a space", "correct horse", "$apr1$abcdefgh$sIQmFnT1CuEXAsyjuXjUX/"},
	// One character of each: the password length drives the bit loop, and a
	// length of 1 takes the zero-byte branch on its only pass.
	{"one character of each", "a", "$apr1$1$Yegk67yDYwZqJvbovPjB10"},
	{"salt openssl generated", "no salt given", "$apr1$oXYNWLEc$uJxNHFrDLcme960LYN0.C0"},
	{"the empty password", "", "$apr1$abcdefgh$L.PT565ESX4Tp2bqNs7Ie."},
}

func TestAPR1Encode(t *testing.T) {
	t.Parallel()

	for _, tc := range apr1Vectors {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			salt, _, ok := apr1Split(tc.stored)
			if !ok {
				t.Fatalf("apr1Split(%q) refused a vector", tc.stored)
			}
			if got := apr1Encode(tc.password, salt); got != tc.stored {
				t.Errorf("apr1Encode(%q, %q) = %q, want openssl's %q", tc.password, salt, got, tc.stored)
			}
		})
	}
}

func TestAPR1Verify(t *testing.T) {
	t.Parallel()

	for _, tc := range apr1Vectors {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if !APR1Verify(tc.password, tc.stored) {
				t.Errorf("APR1Verify(%q, %q) = false, want true", tc.password, tc.stored)
			}
			if APR1Verify(tc.password+"x", tc.stored) {
				t.Errorf("APR1Verify accepted %q against the hash of %q", tc.password+"x", tc.password)
			}
		})
	}
}

// TestAPR1VerifyRefuses covers what is not this format. A value the split
// refuses costs no derivation, which is the same property PasswordVerify has
// for a hash it cannot read.
func TestAPR1VerifyRefuses(t *testing.T) {
	t.Parallel()

	for _, stored := range []string{
		"",
		"plaintext",
		"$apr1$",
		"$apr1$abcdefgh",  // no tag
		"$apr1$abcdefgh$", // empty tag
		"$apr1$toolongasalt$sIQmFnT1CuEXAsyjuXjUX/",        // salt over eight characters
		"$1$abcdefgh$XD5rCJDbNIrOSbHkWEWmy.",               // md5-crypt, not apache's
		"$2y$10$Wq8K7fFvTfGZ6Qf0i3bHhuDG0VH1v8sKoKZBHP7X9", // bcrypt
	} {
		t.Run(stored, func(t *testing.T) {
			t.Parallel()

			if IsAPR1(stored) {
				t.Errorf("isAPR1(%q) = true, want false", stored)
			}
			if APR1Verify("hunter2hunter2", stored) {
				t.Errorf("APR1Verify accepted %q", stored)
			}
		})
	}
}

// TestAPR1EncodeTruncatesTheSalt pins apr_md5_encode's own handling: it keeps
// the first eight bytes of a longer salt, so a hash written with one verifies
// against the truncation. The refusal of such an encoded form belongs to
// apr1Split; this covers the derivation alone.
func TestAPR1EncodeTruncatesTheSalt(t *testing.T) {
	t.Parallel()

	long := apr1Encode("hunter2hunter2", "Zx1kLm9qTRUNCATED")
	short := apr1Encode("hunter2hunter2", "Zx1kLm9q")
	if long != short {
		t.Errorf("apr1Encode with a 17 character salt = %q, want the 8 character answer %q", long, short)
	}
}

// BenchmarkAPR1Verify is the cost of one guess against this format, which is
// the argument docs/security.md makes for `htpasswd -B`. The bcrypt and argon2
// rows of docs/reference/extensions/hashing.md are the comparison.
func BenchmarkAPR1Verify(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if !APR1Verify("hunter2hunter2", "$apr1$Zx1kLm9q$BwLItAYS8lPw/bCsIXHax1") {
			b.Fatal("verify failed")
		}
	}
}
