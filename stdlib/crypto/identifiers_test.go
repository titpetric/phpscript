package crypto

import (
	"strings"
	"testing"
)

// TestEncodeCrockford pins the ULID text encoding to known vectors: the
// all-zero value is 26 zeros, the all-ones value is the spec's largest
// "7ZZZ..." (the leading 7 is the two-bit pad), and a single trailing one
// lands in the last character.
func TestEncodeCrockford(t *testing.T) {
	t.Parallel()
	var zero [16]byte
	if got := encodeCrockford(zero); got != "00000000000000000000000000" {
		t.Errorf("zero value = %q", got)
	}

	var max [16]byte
	for i := range max {
		max[i] = 0xFF
	}
	if got := encodeCrockford(max); got != "7ZZZZZZZZZZZZZZZZZZZZZZZZZ" {
		t.Errorf("max value = %q", got)
	}

	var one [16]byte
	one[15] = 1
	if got := encodeCrockford(one); got != "00000000000000000000000001" {
		t.Errorf("one = %q", got)
	}
}

// TestULIDFrom pins the derived ids to vectors, because they are primary keys:
// changing the digest, or how many of its bytes the encoder takes, renames
// every user in every table that stored one and the rows stop resolving. The
// empty seed sits in the table for the same reason, since a store with an
// unnamed record would otherwise derive a value that moved without notice.
func TestULIDFrom(t *testing.T) {
	t.Parallel()

	for seed, want := range map[string]string{
		"admin":  "4CD5VEBDA10GAVVT88QN6YW5EZ",
		"reader": "1X150SCJN3XFEB036EYP5HQCWS",
		"a":      "6AJY0H5JGVQQ5FNGHHPED27Q2D",
		"":       "73P324567W3GA9NYZMS2CPZE94",
	} {
		if got := ULIDFrom(seed); got != want {
			t.Errorf("ULIDFrom(%q) = %q, want %q", seed, got, want)
		}
	}
}

// TestULIDFromShape covers what a caller relies on without reading the
// vectors: the width and the alphabet are a ulid()'s, so a derived key and a
// generated one are the same kind of column, and one seed answers one id.
func TestULIDFromShape(t *testing.T) {
	t.Parallel()

	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	id := ULIDFrom("admin")
	if len(id) != 26 {
		t.Errorf("ULIDFrom(%q) = %q, want 26 characters", "admin", id)
	}
	if strings.ContainsAny(id, "ILOU") {
		t.Errorf("ULIDFrom(%q) = %q, which is not Crockford base32", "admin", id)
	}
	for _, c := range id {
		if !strings.ContainsRune(alphabet, c) {
			t.Errorf("ULIDFrom(%q) = %q, which carries %q", "admin", id, c)
		}
	}
	if again := ULIDFrom("admin"); again != id {
		t.Errorf("ULIDFrom(%q) = %q then %q, want one answer", "admin", id, again)
	}
	if other := ULIDFrom("admin2"); other == id {
		t.Errorf("ULIDFrom(%q) and ULIDFrom(%q) are both %q", "admin", "admin2", id)
	}
}
