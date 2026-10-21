package core

import (
	"reflect"
	"testing"
)

// TestArrayFindStrict pins the comparison in_array and array_search share: the
// loose path is phpval.Compare, so PHP 8's "a non-numeric string does not equal
// 0" holds, and the strict path compares the Go type first.
func TestArrayFindStrict(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		needle  any
		hay     any
		strict  bool
		wantKey any
		wantOK  bool
	}{
		{name: "numeric string matches int", needle: "1", hay: []any{int64(1)}, wantKey: int64(0), wantOK: true},
		{name: "numeric string is not identical to int", needle: "1", hay: []any{int64(1)}, strict: true},
		{name: "zero does not match a word", needle: int64(0), hay: []any{"a"}},
		{name: "string key is returned", needle: "b", hay: map[string]any{"y": "b"}, wantKey: "y", wantOK: true},
		{name: "int and float differ under strict", needle: int64(1), hay: []any{1.0}, strict: true},
		{name: "nested array compares by value", needle: []any{"a"}, hay: []any{[]any{"a"}}, strict: true, wantKey: int64(0), wantOK: true},
		{name: "null only matches null under strict", needle: nil, hay: []any{"", nil}, strict: true, wantKey: int64(1), wantOK: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, ok := arrayFind(tc.needle, tc.hay, tc.strict)
			if ok != tc.wantOK {
				t.Fatalf("found = %v, want %v", ok, tc.wantOK)
			}
			if ok && !reflect.DeepEqual(key, tc.wantKey) {
				t.Errorf("key = %#v, want %#v", key, tc.wantKey)
			}
		})
	}
}
