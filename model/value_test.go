package model

import (
	"reflect"
	"testing"
)

// TestArrayItemValue covers the already-evaluated array entry the transpiled
// __array and __pair helpers produce. Key is nil for a list-style append, so
// folding a mixed list into an *Array appends those entries at the next integer
// index and sets the rest by key.
//
// That is PHP's numbering for `[1, "k" => 2, 3]`: a string key takes no integer
// position, so the third entry lands at 1.
func TestArrayItemValue(t *testing.T) {
	items := []ArrayItemValue{
		{Val: "first"},
		{Key: "k", Val: "v"},
		{Val: "second"},
	}

	out := NewArraySize(len(items))
	for _, item := range items {
		if item.Key == nil {
			out.Append(item.Val)
			continue
		}
		out.Set(item.Key, item.Val)
	}

	if got, want := out.Keys(), []any{int64(0), "k", int64(1)}; !reflect.DeepEqual(got, want) {
		t.Errorf("the folded array's keys are %v, want %v", got, want)
	}
	for key, want := range map[any]any{int64(0): "first", "k": "v", int64(1): "second"} {
		got, ok := out.Get(key)
		if !ok {
			t.Errorf("nothing is held under %v", key)
			continue
		}
		if got != want {
			t.Errorf("%v holds %v, want %v", key, got, want)
		}
	}

	// An integer key is a key and no append: `[2 => "x"]` sets the position it
	// names, so the next append follows it.
	explicit := NewArray()
	explicit.Set(int64(2), "x")
	explicit.Append("y")
	if got, want := explicit.Keys(), []any{int64(2), int64(3)}; !reflect.DeepEqual(got, want) {
		t.Errorf("the keys after an explicit index are %v, want %v", got, want)
	}
}
