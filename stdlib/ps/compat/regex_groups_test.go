package compat

import (
	"testing"
	"unicode/utf8"
)

// TestMatchOffsetStartsAtBytes covers the other direction: PHP's $offset is a
// byte offset, and it has to keep meaning that when the match runs on the
// backtracking engine, which the non-zero offset itself selects.
func TestMatchOffsetStartsAtBytes(t *testing.T) {
	t.Parallel()
	const subject = "\xc3\xa4b\xc3\xa4b" // "äbäb": the second b is at byte 5

	re, err := compilePCRE("/b/u")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got := re.findIndex(subject, 3); len(got) < 2 || got[0] != 5 {
		t.Errorf("findIndex(subject, 3) = %v, want a match at byte 5", got)
	}
	if got := re.findIndex(subject, utf8.RuneCountInString(subject)); got != nil {
		t.Errorf("findIndex past the last b = %v, want no match", got)
	}
}
