package core

import (
	"errors"
	"reflect"
	"testing"
)

// TestPHPKsort covers ksort keeping every key attached to its value while the
// entries move. sort() next door throws the keys away, so the two families
// cannot share one walk.
func TestPHPKsort(t *testing.T) {
	array := setArray("b", int64(2), "a", int64(1), "c", int64(3))

	ok, err := phpKsort(array)
	if err != nil {
		t.Fatalf("ksort: %v", err)
	}
	if !ok {
		t.Error("ksort() = false on an array")
	}

	want := []any{"a", int64(1), "b", int64(2), "c", int64(3)}
	if got := entries(array); !reflect.DeepEqual(got, want) {
		t.Errorf("ksort left %v, want %v", got, want)
	}
}

// TestPHPKrsort covers ksort's descending spelling, which swaps the operands of
// one comparison and shares everything else.
func TestPHPKrsort(t *testing.T) {
	array := setArray("a", int64(1), "c", int64(3), "b", int64(2))

	if _, err := phpKrsort(array); err != nil {
		t.Fatalf("krsort: %v", err)
	}

	want := []any{"c", int64(3), "b", int64(2), "a", int64(1)}
	if got := entries(array); !reflect.DeepEqual(got, want) {
		t.Errorf("krsort left %v, want %v", got, want)
	}
}

// TestPHPAsort covers asort ordering by value with the keys carried along, so a
// score table sorts without losing the name against each score.
func TestPHPAsort(t *testing.T) {
	array := setArray("x", int64(3), "y", int64(1), "z", int64(2))

	if _, err := phpAsort(array); err != nil {
		t.Fatalf("asort: %v", err)
	}

	want := []any{"y", int64(1), "z", int64(2), "x", int64(3)}
	if got := entries(array); !reflect.DeepEqual(got, want) {
		t.Errorf("asort left %v, want %v", got, want)
	}
}

// TestPHPArsort covers asort's descending spelling.
func TestPHPArsort(t *testing.T) {
	array := setArray("x", int64(1), "y", int64(3), "z", int64(2))

	if _, err := phpArsort(array); err != nil {
		t.Fatalf("arsort: %v", err)
	}

	want := []any{"y", int64(3), "z", int64(2), "x", int64(1)}
	if got := entries(array); !reflect.DeepEqual(got, want) {
		t.Errorf("arsort left %v, want %v", got, want)
	}
}

// TestSortEntries covers the rebuild step, and the divergence from PHP these six
// functions carry.
//
// An integer-keyed array is written back with Set, never Append: appending would
// hand out fresh keys and turn a key-preserving sort into sort(). A native Go
// slice is refused, because a slice has no keys to preserve and sorting one
// would be sort() under another name.
func TestSortEntries(t *testing.T) {
	sparse := setArray(int64(5), "five", int64(2), "two", int64(9), "nine")

	if _, err := phpKsort(sparse); err != nil {
		t.Fatalf("ksort: %v", err)
	}
	want := []any{int64(2), "two", int64(5), "five", int64(9), "nine"}
	if got := entries(sparse); !reflect.DeepEqual(got, want) {
		t.Errorf("ksort renumbered the keys to %v, want %v", got, want)
	}

	for _, value := range []any{[]string{"b", "a"}, []any{int64(2), int64(1)}, nil} {
		if _, err := phpAsort(value); err == nil {
			t.Errorf("asort(%T) was accepted; only a *model.Array has keys to keep", value)
		}
	}
}

// TestCallbackLess covers adapting a script comparator to the boolean
// sort.SliceStable takes. A comparator that raises reads as "not less", so a
// failing callback leaves the order it saw and aborts no sort mid-way.
func TestCallbackLess(t *testing.T) {
	descending := func(args ...any) (any, error) {
		return args[1].(int64) - args[0].(int64), nil
	}
	if !callbackLess(descending, int64(2), int64(1)) {
		t.Error("a comparator answering a negative number read as not less")
	}
	if callbackLess(descending, int64(1), int64(2)) {
		t.Error("a comparator answering a positive number read as less")
	}
	if callbackLess(descending, int64(1), int64(1)) {
		t.Error("a comparator answering zero read as less")
	}

	failing := func(_ ...any) (any, error) { return nil, errors.New("no") }
	if callbackLess(failing, int64(1), int64(2)) {
		t.Error("a comparator that raised read as less")
	}
}
