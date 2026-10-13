package model

import (
	"testing"
)

// TestByRefArg covers the argument positions a builtin writes back through,
// counted from zero the way PHP counts them: preg_match's third argument is the
// match array, preg_replace_callback's fifth is the replacement count. A
// namespaced file carries both the qualified call name and the global name it
// falls back to, so either spelling has to match.
//
// exec's $output, its second argument, is absent from the table: an array is
// shared, so the binding appends into the one the caller passed and reaches the
// same observable result. Its exit status, the third, has no such route back.
func TestByRefArg(t *testing.T) {
	for _, tc := range []struct {
		name     string
		fallback string
		index    int
		want     bool
	}{
		{name: "preg_match", index: 2, want: true},
		{name: "preg_match", index: 1},
		{name: "preg_match_all", index: 2, want: true},
		{name: "preg_replace_callback", index: 4, want: true},
		{name: "preg_replace_callback", index: 2},
		{name: "parse_str", index: 1, want: true},
		{name: "exec", index: 2, want: true},
		{name: "exec", index: 1},
		{name: "system", index: 1, want: true},
		{name: "passthru", index: 1, want: true},
		{name: `App\preg_match`, fallback: "preg_match", index: 2, want: true},
		{name: `App\preg_match`, fallback: "preg_match", index: 0},
		{name: "strlen", index: 0},
	} {
		if got := ByRefArg(tc.name, tc.fallback, tc.index); got != tc.want {
			t.Errorf("ByRefArg(%q, %q, %d) = %v, want %v", tc.name, tc.fallback, tc.index, got, tc.want)
		}
	}
}
