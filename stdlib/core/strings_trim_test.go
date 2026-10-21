package core

import (
	"testing"
)

// TestTrimSides covers the three trims one constructor builds: the default
// character list is php's five whitespace bytes plus NUL, a call naming its
// own list replaces that list, and each side strips only where it is asked to.
func TestTrimSides(t *testing.T) {
	t.Parallel()
	both := phpTrim(trimBoth, defaultTrimChars)
	left := phpTrim(trimLeft, defaultTrimChars)
	right := phpTrim(trimRight, defaultTrimChars)

	if got := both("  x\t\n"); got != "x" {
		t.Errorf("trim = %q, want x", got)
	}
	if got := left("  x  "); got != "x  " {
		t.Errorf("ltrim = %q, want the right side kept", got)
	}
	if got := right("  x  "); got != "  x" {
		t.Errorf("rtrim = %q, want the left side kept", got)
	}
	if got := both("xxhelloxx", "x"); got != "hello" {
		t.Errorf("trim with a named list = %q, want hello", got)
	}
	// A NUL is in the default list, which is what makes a C string trim.
	if got := both("x\x00"); got != "x" {
		t.Errorf("trim of a trailing NUL = %q, want x", got)
	}
}

// TestCharMaskRanges covers the "a..z" spelling the trim family's character
// list takes: the range includes both ends, a reversed range is no range, and
// a trailing ".." is two literal dots.
func TestCharMaskRanges(t *testing.T) {
	t.Parallel()
	mask := charMask("a..e")
	for _, c := range []byte{'a', 'c', 'e'} {
		if !mask[c] {
			t.Errorf("charMask(a..e) leaves %q out", c)
		}
	}
	if mask['f'] {
		t.Error("charMask(a..e) includes f")
	}

	literal := charMask("a.")
	if !literal['a'] || !literal['.'] {
		t.Error("charMask reads a short list as a range")
	}
}
