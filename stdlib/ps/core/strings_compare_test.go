package core

import (
	"math"
	"testing"
)

// TestCompareBytes covers the one comparison strcmp, strcasecmp, strncmp and
// strncasecmp differ only in the arguments to.
//
// The number is the difference between the first two bytes that disagree, with
// no normalisation to -1/0/1, because a fixture recorded from php pins it:
// strcmp("A", "a") is -32 there. Bytes compare unsigned and "\xff" sorts after
// "\x01" by 254. A tie broken by length normalises, php reading strcmp("a", "")
// as 1 whatever the tail holds.
func TestCompareBytes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		a, b  string
		limit int
		fold  bool
		want  int64
	}{
		{name: "strcmp folds nothing", a: "A", b: "a", limit: wholeString, want: -32},
		{name: "strcmp of a prefix normalises", a: "a", b: "abcdefgh", limit: wholeString, want: -1},
		{name: "strcmp against the empty string", a: "a", b: "", limit: wholeString, want: 1},
		{name: "bytes compare unsigned", a: "\xff", b: "\x01", limit: wholeString, want: 254},
		{name: "equal strings", a: "same", b: "same", limit: wholeString, want: 0},
		{name: "strcasecmp folds A-Z", a: "HELLO", b: "hello", limit: wholeString, fold: true, want: 0},
		{name: "strcasecmp reaches no accented letter", a: "\xc3\x84", b: "\xc3\xa4", limit: wholeString, fold: true, want: -32},
		{name: "strncmp stops at its limit", a: "abc", b: "abd", limit: 2, want: 0},
		{name: "strncmp past both ends still ties on length", a: "abc", b: "abd", limit: 10, want: -1},
		{name: "strncmp of nothing", a: "abc", b: "xyz", limit: 0, want: 0},
		{name: "strncasecmp stops at its limit", a: "ABCx", b: "abcY", limit: 3, fold: true, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := compareBytes(tc.a, tc.b, tc.limit, tc.fold); got != tc.want {
				t.Errorf("compareBytes(%q, %q, %d, %v) = %d, want %d", tc.a, tc.b, tc.limit, tc.fold, got, tc.want)
			}
		})
	}
}

// TestClampLength covers reading strncmp's $length. PHP 8 raises a ValueError
// below zero; these bindings clamp to zero, where php returns 0 for the call
// anyway. A length wider than any string this process holds bounds nothing and
// it saturates at the largest int.
func TestClampLength(t *testing.T) {
	t.Parallel()
	maxInt := int(^uint(0) >> 1)

	for _, tc := range []struct {
		length int64
		want   int
	}{
		{length: -1, want: 0},
		{length: 0, want: 0},
		{length: 3, want: 3},
		{length: math.MaxInt64, want: maxInt},
	} {
		if got := clampLength(tc.length); got != tc.want {
			t.Errorf("clampLength(%d) = %d, want %d", tc.length, got, tc.want)
		}
	}
}

// TestLowerASCII covers the fold the case-insensitive comparisons use: A-Z
// alone. These functions walk bytes and decode no rune and a byte above
// 127 passes through unchanged.
func TestLowerASCII(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in, want byte
	}{
		{in: 'A', want: 'a'},
		{in: 'Z', want: 'z'},
		{in: 'a', want: 'a'},
		{in: '7', want: '7'},
		{in: '[', want: '['},
		{in: 0xc4, want: 0xc4},
	} {
		if got := lowerASCII(tc.in); got != tc.want {
			t.Errorf("lowerASCII(%#x) = %#x, want %#x", tc.in, got, tc.want)
		}
	}
}
