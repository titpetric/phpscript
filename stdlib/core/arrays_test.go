package core

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestArrayKeyExistsHoldsNull covers the difference array_key_exists() answers
// and isset() does not: a key whose value is null is a key the array holds.
func TestArrayKeyExistsHoldsNull(t *testing.T) {
	t.Parallel()
	array := model.NewArray()
	array.Set("a", nil)

	if !phpArrayKeyExists("a", array) {
		t.Error("array_key_exists is false for a key holding null")
	}
	if phpArrayKeyExists("b", array) {
		t.Error("array_key_exists is true for a key nothing set")
	}
	if _, ok := arrayHasKey(array, "a"); !ok {
		t.Error("arrayHasKey is false for a key holding null")
	}
}

// TestArraySumPromotes covers array_sum(): a float or a numeric string anywhere
// makes the sum a float, and a list of integers stays an integer.
func TestArraySumPromotes(t *testing.T) {
	t.Parallel()
	if got := phpArraySum([]any{int64(1), int64(2)}); got != int64(3) {
		t.Errorf("array_sum of integers = %#v, want int64(3)", got)
	}
	if got := phpArraySum([]any{int64(1), 2.5, "3"}); got != 6.5 {
		t.Errorf("array_sum with a float = %#v, want 6.5", got)
	}
	if got := phpArraySum(model.NewArray()); got != int64(0) {
		t.Errorf("array_sum of an empty array = %#v, want int64(0)", got)
	}
}

// TestArrayFlipAndReverse covers the two rearrangements: flip swaps keys for
// values, and reverse renumbers a list unless the call asks to keep the keys,
// which is what php's second argument decides.
func TestArrayFlipAndReverse(t *testing.T) {
	t.Parallel()
	source := model.NewArray()
	source.Set("a", int64(1))
	source.Set("b", int64(2))

	flipped := phpArrayFlip(source)
	if value, _ := flipped.Get(int64(1)); value != "a" {
		t.Errorf("array_flip put %#v under 1, want a", value)
	}

	// A list of integer keys reverses into a Go slice, which is the cheapest
	// shape for the list it stays (docs/allocation-performance.md).
	reversed, ok := phpArrayReverse([]any{int64(1), int64(2), int64(3)}).([]any)
	if !ok {
		t.Fatalf("array_reverse of a list answered %T, want a slice", reversed)
	}
	if reversed[0] != int64(3) {
		t.Errorf("array_reverse put %#v first, want 3", reversed[0])
	}

	kept, ok := phpArrayReverse(source, true).(*model.Array)
	if !ok {
		t.Fatalf("array_reverse with preserve_keys answered %T", kept)
	}
	if value, _ := kept.Get("b"); value != int64(2) {
		t.Errorf("array_reverse with preserve_keys dropped the string key")
	}
}
