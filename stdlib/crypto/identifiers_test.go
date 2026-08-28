package crypto

import "testing"

// TestEncodeCrockford pins the ULID text encoding to known vectors: the
// all-zero value is 26 zeros, the all-ones value is the spec's maximum
// "7ZZZ..." (the leading 7 is the two-bit pad), and a single trailing one
// lands in the last character.
func TestEncodeCrockford(t *testing.T) {
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
