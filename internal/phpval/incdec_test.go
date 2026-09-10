package phpval

import (
	"math"
	"reflect"
	"testing"
)

// The expected values are php 8.5's, captured with var_dump; see
// tests/fixtures/arithmetic/increment_decrement.phpt for the end-to-end run
// of the same table.
func TestIncrement(t *testing.T) {
	cases := []struct {
		in   any
		want any
	}{
		{int64(5), int64(6)},
		{1.5, 2.5},
		{nil, int64(1)},
		{true, true},
		{false, false},
		{"5", int64(6)},
		{"5.5", 6.5},
		{"1e2", 101.0},
		{" 5", int64(6)},
		{"a", "b"},
		{"z", "aa"},
		{"Az", "Ba"},
		{"a9", "b0"},
		{"Zz9", "AAa0"},
		{"", "1"},
		{"a-b", "a-c"},
		{"a-z", "a-a"},
		{"-z", "-a"},
		{"9z", "10a"},
		{" z", " a"},
		{"ab-", "ab-"},
		{int64(math.MaxInt64), float64(math.MaxInt64) + 1},
	}
	for _, tc := range cases {
		if got := Increment(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Increment(%#v) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}

func TestDecrement(t *testing.T) {
	cases := []struct {
		in   any
		want any
	}{
		{int64(5), int64(4)},
		{2.5, 1.5},
		{nil, nil},
		{true, true},
		{"6", int64(5)},
		{"5.5", 4.5},
		{"a", "a"},
		{"", int64(-1)},
		{int64(math.MinInt64), float64(math.MinInt64) - 1},
	}
	for _, tc := range cases {
		if got := Decrement(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Decrement(%#v) = %#v, want %#v", tc.in, got, tc.want)
		}
	}
}
