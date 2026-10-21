package core

import (
	"testing"
)

// TestASCIICaseFolding covers the two folds this package writes itself rather
// than taking from strings: php's strtolower and strtoupper fold the ASCII
// range alone, so a byte over 0x7f travels unchanged and a UTF-8 sequence
// survives a round trip through either.
func TestASCIICaseFolding(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		in    string
		lower string
		upper string
	}{
		{"Hello", "hello", "HELLO"},
		{"ÄÖÜ", "ÄÖÜ", "ÄÖÜ"},
		{"a1_Z", "a1_z", "A1_Z"},
		{"", "", ""},
	} {
		if got := asciiLower(test.in); got != test.lower {
			t.Errorf("asciiLower(%q) = %q, want %q", test.in, got, test.lower)
		}
		if got := asciiUpper(test.in); got != test.upper {
			t.Errorf("asciiUpper(%q) = %q, want %q", test.in, got, test.upper)
		}
	}
}

// TestUcwordsSeparators covers ucwords(): the default separators are the five
// whitespace bytes, and a call naming its own replaces them, so a hyphen
// capitalises only where the call asks for it.
func TestUcwordsSeparators(t *testing.T) {
	t.Parallel()
	if got := phpUcwords("hello big-world"); got != "Hello Big-world" {
		t.Errorf("ucwords = %q, want the default separators", got)
	}
	if got := phpUcwords("hello big-world", "-"); got != "Hello big-World" {
		t.Errorf("ucwords with a named separator = %q", got)
	}
	if got := phpUcfirst("étude"); got != "étude" {
		t.Errorf("ucfirst over a multi-byte first rune = %q, want it unchanged", got)
	}
	if got := phpLcfirst("Hello"); got != "hello" {
		t.Errorf("lcfirst = %q, want hello", got)
	}
}
