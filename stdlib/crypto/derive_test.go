package crypto

import (
	"strings"
	"testing"
)

// TestDerive covers both derivations through the one entry point
// password_hash() uses: a bcrypt hash carries php's revision marker and the
// cost it was written at, an argon2 hash carries the PHC string form, and each
// one verifies.
func TestDerive(t *testing.T) {
	for _, tc := range []struct {
		name   string
		p      params
		prefix string
	}{
		{"bcrypt", params{algo: algoBcrypt, cost: bcryptMinCost}, "$2y$04$"},
		{"argon2id", params{algo: algoArgon2id, memory: 8, time: 1, lanes: 1}, "$argon2id$v=19$"},
		{"argon2i", params{algo: algoArgon2i, memory: 8, time: 1, lanes: 1}, "$argon2i$v=19$"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := derive("correct horse", tc.p)
			if err != nil {
				t.Fatalf("derive() = %v", err)
			}
			if !strings.HasPrefix(hash, tc.prefix) {
				t.Errorf("derive() = %q, want a hash starting %q", hash, tc.prefix)
			}
			if !passwordVerify("correct horse", hash) {
				t.Error("the hash it wrote does not verify")
			}
			if passwordVerify("wrong horse", hash) {
				t.Error("a different password verified against it")
			}
		})
	}
}

// TestBcryptVerify covers the bcrypt half on a hash written at php's revision
// marker, which x/crypto refuses until the prefix is rewritten, and the cost
// read back off it.
func TestBcryptVerify(t *testing.T) {
	hash, err := derive("hunter2", params{algo: algoBcrypt, cost: bcryptMinCost})
	if err != nil {
		t.Fatalf("derive() = %v", err)
	}
	if !strings.HasPrefix(hash, phpPrefix) {
		t.Fatalf("derive() = %q, want php's %q marker", hash, phpPrefix)
	}

	if !bcryptVerify("hunter2", hash) {
		t.Error("bcryptVerify() = false on the password the hash was written from")
	}
	if bcryptVerify("hunter3", hash) {
		t.Error("bcryptVerify() = true on a different password")
	}

	cost, err := bcryptCost(hash)
	if err != nil {
		t.Fatalf("bcryptCost() = %v", err)
	}
	if cost != bcryptMinCost {
		t.Errorf("bcryptCost() = %d, want %d", cost, bcryptMinCost)
	}
	if _, err := bcryptCost("not-a-bcrypt-hash"); err == nil {
		t.Error("bcryptCost() read a cost out of a string that is not a hash")
	}
}

// TestArgon2Key covers the two variants being different derivations. The same
// password, salt and work factors under argon2i and argon2id answer different
// tags, so a hash written as one cannot verify as the other.
func TestArgon2Key(t *testing.T) {
	p := params{algo: algoArgon2id, memory: 8, time: 1, lanes: 1}
	salt := []byte("0123456789abcdef")

	id := argon2Key(p, []byte("hunter2"), salt, argon2KeyLen)
	if len(id) != argon2KeyLen {
		t.Fatalf("argon2Key() wrote %d bytes, want %d", len(id), argon2KeyLen)
	}
	if string(argon2Key(p, []byte("hunter2"), salt, argon2KeyLen)) != string(id) {
		t.Error("the same inputs answered two different tags")
	}

	p.algo = algoArgon2i
	if string(argon2Key(p, []byte("hunter2"), salt, argon2KeyLen)) == string(id) {
		t.Error("argon2i and argon2id answered the same tag")
	}
}
