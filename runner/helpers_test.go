package runner

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestHelperConcat covers `.`, which renders both operands the way PHP renders
// them in a string context and a concatenation of an int and a bool spells
// them as echo spells them.
func TestHelperConcat(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		a, b any
		want string
	}{
		{"a", "b", "ab"},
		{"n=", int64(7), "n=7"},
		{true, false, "1"},
		{nil, "x", "x"},
		{2.5, "kg", "2.5kg"},
	} {
		if got := helperConcat(tc.a, tc.b); got != tc.want {
			t.Errorf("helperConcat(%#v, %#v) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestHelperArray covers `array()` and `[...]`: an unkeyed item takes the next
// integer index, a keyed one lands under its normalised key, and the two
// interleave the way PHP interleaves them.
func TestHelperArray(t *testing.T) {
	t.Parallel()
	out := helperArray(
		helperPair(nil, "first"),
		helperPair("name", "ada"),
		helperPair(nil, "second"),
		helperPair("3", "fourth"),
	)

	for key, want := range map[any]any{
		int64(0): "first",
		"name":   "ada",
		int64(1): "second",
		int64(3): "fourth",
	} {
		value, ok := out.Get(key)
		if !ok {
			t.Errorf("the array has no entry under %#v", key)
			continue
		}
		if value != want {
			t.Errorf("the array holds %#v under %#v, want %#v", value, key, want)
		}
	}
	if out.Len() != 4 {
		t.Errorf("the array holds %d entries, want 4", out.Len())
	}
}

// TestHelperIndex covers `base[idx]` over every container a script can index. A
// missing key answers nil, PHP's forgiving access, and raises nothing. A string
// offset is the byte at that position and it agrees with strlen and with the
// offsets strpos reports, and a negative offset counts from the end.
func TestHelperIndex(t *testing.T) {
	t.Parallel()
	array := model.NewArray()
	array.Set("name", "ada")
	array.Set(int64(2), "third")

	for _, tc := range []struct {
		name string
		base any
		idx  any
		want any
	}{
		{"an array by name", array, "name", "ada"},
		{"an array by a numeric string", array, "2", "third"},
		{"a missing key", array, "absent", nil},
		{"a string offset", "hello", int64(1), "e"},
		{"a negative string offset", "hello", int64(-1), "o"},
		{"a string offset past the end", "hello", int64(10), ""},
		{"a string offset before the start", "hello", int64(-10), ""},
		{"bytes", []byte("hello"), int64(0), "h"},
		{"nil", nil, "name", nil},
		{"a Go map", map[string]int{"n": 7}, "n", 7},
		{"a Go map by an int key", map[int]string{3: "third"}, int64(3), "third"},
		{"a Go map miss", map[string]int{"n": 7}, "absent", nil},
		{"a Go slice", []string{"a", "b"}, int64(1), "b"},
		{"a Go slice out of range", []string{"a"}, int64(5), nil},
		{"a value with no index", int64(1), int64(0), nil},
	} {
		if got := helperIndex(tc.base, tc.idx); got != tc.want {
			t.Errorf("%s: helperIndex() = %#v, want %#v", tc.name, got, tc.want)
		}
	}
}
