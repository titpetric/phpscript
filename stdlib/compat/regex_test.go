package compat

import (
	"testing"
	"unicode/utf8"
)

// TestRuneOffsetsByteOffset covers the conversion regexp2 needs and RE2 does
// not. It is the one part of the offset work an ASCII fixture cannot see: rune
// index and byte offset are the same number until the subject holds a
// multibyte character, so a missing conversion looks correct everywhere else.
func TestRuneOffsetsByteOffset(t *testing.T) {
	for _, test := range []struct {
		name    string
		subject string
		want    []int // byte offset per rune index, one entry past the end
	}{
		{
			name:    "ascii indexes are byte offsets",
			subject: "abc",
			want:    []int{0, 1, 2, 3},
		},
		{
			name:    "two-byte character shifts everything after it",
			subject: "\xc3\xa4bc",
			want:    []int{0, 2, 3, 4},
		},
		{
			name:    "three- and four-byte characters",
			subject: "a\xe2\x82\xac\xf0\x9f\x92\xa9z",
			want:    []int{0, 1, 4, 8, 9},
		},
		{
			name:    "empty subject has only the end",
			subject: "",
			want:    []int{0},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			offsets := newRuneOffsets(test.subject)
			for i, want := range test.want {
				if got := offsets.byteOffset(i); got != want {
					t.Errorf("byteOffset(%d) = %d, want %d", i, got, want)
				}
			}
			// A rune index past the end is the end of the subject, which is
			// what the end of a match on the last rune converts to.
			if got := offsets.byteOffset(len(test.want) + 5); got != len(test.subject) {
				t.Errorf("byteOffset past end = %d, want %d", got, len(test.subject))
			}
			// A group that did not participate stays negative rather than
			// becoming an offset into the subject.
			if got := offsets.byteOffset(-1); got != -1 {
				t.Errorf("byteOffset(-1) = %d, want -1", got)
			}
		})
	}
}

// TestRuneOffsetsASCIIKeepsNoTable pins the fast path: an ASCII subject needs
// no table, since the identity is the conversion.
func TestRuneOffsetsASCIIKeepsNoTable(t *testing.T) {
	if offsets := newRuneOffsets("plain ascii"); offsets.byteAt != nil {
		t.Errorf("ascii subject built a %d entry table", len(offsets.byteAt))
	}
	if offsets := newRuneOffsets("\xc3\xa4"); offsets.byteAt == nil {
		t.Error("multibyte subject built no table")
	}
}

// TestFindIndexReportsByteOffsets covers the conversion where it matters: the
// backtracking engine reports rune indexes, and a script reading
// PREG_OFFSET_CAPTURE has to see the byte offsets PHP reports. The lookahead
// forces the pattern onto that engine; the plain one stays on RE2, and the two
// have to agree.
func TestFindIndexReportsByteOffsets(t *testing.T) {
	const subject = "\xc3\xa4bc" // "äbc": b starts at byte 2, rune 1

	for _, test := range []struct {
		name    string
		pattern string
		want    []int
	}{
		{name: "re2", pattern: "/b/u", want: []int{2, 3}},
		{name: "backtracking", pattern: "/b(?=c)/u", want: []int{2, 3}},
		{name: "backtracking group", pattern: "/(b)(?=c)/u", want: []int{2, 3, 2, 3}},
	} {
		t.Run(test.name, func(t *testing.T) {
			re, err := compilePCRE(test.pattern)
			if err != nil {
				t.Fatalf("compile %q: %v", test.pattern, err)
			}
			got := re.findIndex(subject, 0)
			if len(got) != len(test.want) {
				t.Fatalf("findIndex = %v, want %v", got, test.want)
			}
			for i := range test.want {
				if got[i] != test.want[i] {
					t.Fatalf("findIndex = %v, want %v", got, test.want)
				}
			}
		})
	}
}

// TestMatchOffsetStartsAtBytes covers the other direction: PHP's $offset is a
// byte offset, and it has to keep meaning that when the match runs on the
// backtracking engine, which the non-zero offset itself selects.
func TestMatchOffsetStartsAtBytes(t *testing.T) {
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
