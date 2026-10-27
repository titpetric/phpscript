package compat

import (
	"reflect"
	"testing"
)

// TestPregQuoteEscapes covers preg_quote(): every PCRE metacharacter comes
// back escaped, and so does a delimiter the call names and the result sits
// inside a pattern written with that delimiter.
func TestPregQuoteEscapes(t *testing.T) {
	t.Parallel()
	if got := phpPregQuote("a.b*c"); got != `a\.b\*c` {
		t.Errorf("preg_quote = %q, want a\\.b\\*c", got)
	}
	if got := phpPregQuote("a/b", "/"); got != `a\/b` {
		t.Errorf("preg_quote with a delimiter = %q, want a\\/b", got)
	}
	if got := phpPregQuote("plain"); got != "plain" {
		t.Errorf("preg_quote of plain text = %q, want it unchanged", got)
	}
}

// TestPregMatchWritesGroups covers what the matches reference receives: the
// whole match first and one entry per group and $m[1] is the first capture.
// The return is the match count php answers, which is 1 or 0 for preg_match.
func TestPregMatchWritesGroups(t *testing.T) {
	t.Parallel()
	cache := newRegexpCache()

	var written any
	got := cache.phpPregMatch(`/(\d+)/`, "x42", func(v any) { written = v }, nil, nil)
	if got != int64(1) {
		t.Fatalf("preg_match = %#v, want 1", got)
	}
	// A match of plain strings travels as []string, the shape a binding
	// returns where every value is one (docs/allocation-performance.md).
	matches, ok := written.([]string)
	if !ok {
		t.Fatalf("the matches reference holds %T, want a string list", written)
	}
	if len(matches) != 2 || matches[0] != "42" || matches[1] != "42" {
		t.Errorf("the matches reference holds %#v, want the match and its group", matches)
	}

	if got := cache.phpPregMatch(`/z/`, "x42", func(v any) { written = v }, nil, nil); got != int64(0) {
		t.Errorf("preg_match with no match = %#v, want 0", got)
	}
}

// TestPregSplitNoEmpty covers PREG_SPLIT_NO_EMPTY, which drops the empty
// piece two adjacent separators leave between them.
func TestPregSplitNoEmpty(t *testing.T) {
	t.Parallel()
	cache := newRegexpCache()

	all := cache.phpPregSplit(`/,/`, "a,,b", nil, nil)
	if got, want := all, []string{"a", "", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("preg_split = %#v, want %#v", got, want)
	}

	kept := cache.phpPregSplit(`/,/`, "a,,b", nil, int64(pregSplitNoEmpty))
	if got, want := kept, []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("preg_split with NO_EMPTY = %#v, want %#v", got, want)
	}
}
