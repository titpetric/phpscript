package core

import (
	"testing"
)

// TestStringSearchPositions covers the four position functions: an offset
// counts bytes and the search starts there, a negative one counts back, the
// reverse pair answers the last match, and the folded pair matches ASCII case.
// A needle the haystack does not hold is false, so the return is any.
//
// An offset past the end answers false here, where php 8 raises a ValueError.
func TestStringSearchPositions(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		got  any
		want any
	}{
		{"from an offset", phpStrpos("hello world", "o", 5), int64(7)},
		{"from the start", phpStrpos("hello world", "o"), int64(4)},
		{"no match", phpStrpos("hello", "z"), false},
		{"folded", phpStripos("HeLLo", "ll"), int64(2)},
		{"last match", phpStrrpos("hello world", "o"), int64(7)},
		{"last match folded", phpStrripos("hello wOrld", "O"), int64(7)},
		{"an offset past the end", phpStrpos("hello", "o", 9), false},
		{"a negative offset counts back", phpStrpos("hello world", "o", -4), int64(7)},
	} {
		if test.got != test.want {
			t.Errorf("%s = %#v, want %#v", test.name, test.got, test.want)
		}
	}
}

// TestSubstrCountWindow covers the offset and length substr_count() takes: the
// window is the part of the haystack counted over, and a count past the window
// is not a count.
func TestSubstrCountWindow(t *testing.T) {
	t.Parallel()
	if got := phpSubstrCount("aaa", "a"); got != 3 {
		t.Errorf("substr_count = %d, want 3", got)
	}
	if got := phpSubstrCount("aaa", "a", int64(1)); got != 2 {
		t.Errorf("substr_count from an offset = %d, want 2", got)
	}
	if got := phpSubstrCount("aaa", "a", int64(0), int64(2)); got != 2 {
		t.Errorf("substr_count over a window = %d, want 2", got)
	}
	// Overlapping needles count once each, as php counts them.
	if got := phpSubstrCount("aaa", "aa"); got != 1 {
		t.Errorf("substr_count of an overlapping needle = %d, want 1", got)
	}
}

// TestStringContainsFamily covers the three php 8 predicates, which pass
// straight through to strings and answer true for an empty needle.
func TestStringContainsFamily(t *testing.T) {
	t.Parallel()
	if !phpStrContains("hello", "ell") || !phpStrStartsWith("hello", "he") || !phpStrEndsWith("hello", "lo") {
		t.Error("a needle the haystack holds answered false")
	}
	if !phpStrContains("hello", "") || !phpStrStartsWith("hello", "") || !phpStrEndsWith("hello", "") {
		t.Error("an empty needle answered false, which php reads as present")
	}
}
