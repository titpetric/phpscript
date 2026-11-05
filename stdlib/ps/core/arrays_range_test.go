package core

import (
	"reflect"
	"testing"
)

// TestRangeShapes covers range(): the ends are inclusive, a descending pair
// counts down with a positive step the way php reads it, a character pair
// answers single-byte strings, and a float anywhere makes every element float.
func TestRangeShapes(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		got  []any
		want []any
	}{
		{"ascending", phpRange(int64(1), int64(4)), []any{int64(1), int64(2), int64(3), int64(4)}},
		{"descending", phpRange(int64(5), int64(1), int64(2)), []any{int64(5), int64(3), int64(1)}},
		{"one element", phpRange(int64(3), int64(3)), []any{int64(3)}},
		{"characters", phpRange("a", "e", int64(2)), []any{"a", "c", "e"}},
		{"a float step", phpRange(int64(1), int64(2), 0.5), []any{1.0, 1.5, 2.0}},
	} {
		if !reflect.DeepEqual(test.got, test.want) {
			t.Errorf("%s = %#v, want %#v", test.name, test.got, test.want)
		}
	}
}

// TestRangeCharacterPairs covers which pairs read as characters: both ends one
// byte, whatever that byte is. php answers strings for range("1", "5") too,
// and reads an end of more than one byte as a number.
func TestRangeCharacterPairs(t *testing.T) {
	t.Parallel()
	if _, _, ok := rangeChars("a", "e"); !ok {
		t.Error("two single letters do not read as a character range")
	}
	if _, _, ok := rangeChars("1", "5"); !ok {
		t.Error("two single digits do not read as a character range, which php answers strings for")
	}
	if _, _, ok := rangeChars("ab", "e"); ok {
		t.Error("a two byte end reads as a character range")
	}
	if _, _, ok := rangeChars(int64(1), int64(5)); ok {
		t.Error("two integers read as a character range")
	}
}
