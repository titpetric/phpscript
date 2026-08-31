package core

import "testing"

// TestFnmatch pins the matcher against php. Every expectation here was produced
// by running the same pattern, subject and flags through php's fnmatch, not
// through this implementation; the flag columns are the reason the table is
// this long, since each one changes a different decision in the loop.
func TestFnmatch(t *testing.T) {
	tests := []struct {
		pattern string
		str     string
		flags   int64
		want    bool
	}{
		// The field-filter case the fixture uses.
		{"post_*", "post_id", 0, true},
		{"post_*", "post_title", 0, true},
		{"post_*", "author_id", 0, false},
		{"post_*", "id", 0, false},
		{"post_?d", "post_id", 0, true},
		{"post_?d", "post_xd", 0, true},

		// Empty pattern and subject.
		{"*", "", 0, true},
		{"", "", 0, true},
		{"", "x", 0, false},

		// A separator is an ordinary byte until FNM_PATHNAME says so.
		{"a*b", "a/x/b", 0, true},
		{"a*b", "a/x/b", fnmPathname, false},
		{"a?b", "a/b", 0, true},
		{"a?b", "a/b", fnmPathname, false},
		{"*.txt", "a/b.txt", 0, true},
		{"*.txt", "a/b.txt", fnmPathname, false},
		{"*/*.txt", "a/b.txt", fnmPathname, true},
		{"**", "a/b", fnmPathname, false},

		// Bracket expressions, including both spellings of negation and
		// the POSIX literal positions.
		{"[!a]bc", "xbc", 0, true},
		{"[!a]bc", "abc", 0, false},
		{"[^a]bc", "xbc", 0, true},
		{"[a-c]*", "b1", 0, true},
		{"[a-c]*", "d1", 0, false},
		{"[]]", "]", 0, true},
		{"[]a]", "a", 0, true},
		{"[a-]", "-", 0, true},
		{"[a-c-e]", "-", 0, true},
		{"[a-c-e]", "d", 0, false},

		// An unterminated '[' is a literal, which is why the first of
		// these fails: the pattern then wants the four bytes "[abc".
		{"[abc", "[", 0, false},
		{"[abc", "a", 0, false},
		{"[abc", "[abc", 0, true},
		{"x[", "x[", 0, true},
		{"[!]", "]", 0, false},

		// Escapes, and FNM_NOESCAPE turning the backslash into a byte.
		{`a\*b`, "a*b", 0, true},
		{`a\*b`, "axb", 0, false},
		{`a\*b`, "a*b", fnmNoescape, false},
		{`a\\b`, `a\b`, 0, true},

		// FNM_PERIOD makes a leading period explicit-only.
		{"*x", ".ax", 0, true},
		{"*x", ".ax", fnmPeriod, false},
		{".*x", ".ax", fnmPeriod, true},
		{"*", ".a", fnmPeriod, false},
		{"?a", ".a", fnmPeriod, false},
		{"[.]a", ".a", fnmPeriod, false},

		// FNM_CASEFOLD folds both sides.
		{"POST_*", "post_id", fnmCasefold, true},
		{"post_*", "POST_ID", fnmCasefold, true},
		{"POST_*", "post_id", 0, false},

		// Backtracking across several stars.
		{"*a*b*c*", "xaybzc", 0, true},
		{"*a*b*c*", "xaybz", 0, false},
		{"a**b", "ab", 0, true},
		{"a**b", "axxb", 0, true},
	}

	for _, test := range tests {
		got := phpFnmatch(test.pattern, test.str, test.flags)
		if got != test.want {
			t.Errorf("fnmatch(%q, %q, %d) = %v, want %v", test.pattern, test.str, test.flags, got, test.want)
		}
	}
}

// TestFnmatchDefaultFlags checks the two-argument form, which is how every
// script calls it: the variadic slot is empty and the mode is zero.
func TestFnmatchDefaultFlags(t *testing.T) {
	if !phpFnmatch("post_*", "post_id") {
		t.Error("fnmatch(post_*, post_id) = false, want true")
	}
	if phpFnmatch("post_*", "author_id") {
		t.Error("fnmatch(post_*, author_id) = true, want false")
	}
}
