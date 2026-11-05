package core

import (
	"reflect"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// setArray builds an array from alternating keys and values and a case table
// reads as the PHP literal it stands for.
func setArray(pairs ...any) *model.Array {
	out := model.NewArray()
	for i := 0; i+1 < len(pairs); i += 2 {
		out.Set(pairs[i], pairs[i+1])
	}
	return out
}

// entries reads an array back as alternating keys and values, the shape
// setArray takes and a test spells an expected array the same way as an input.
func entries(a *model.Array) []any {
	var out []any
	a.Range(func(k, v any) bool {
		out = append(out, k, v)
		return true
	})
	return out
}

// TestValueList covers reading the values of an array in order. array_combine
// pairs two arrays at the same offset, which a range over one of them cannot
// do.
func TestValueList(t *testing.T) {
	t.Parallel()
	got := valueList(setArray("b", int64(2), "a", int64(1)))
	if want := []any{int64(2), int64(1)}; !reflect.DeepEqual(got, want) {
		t.Errorf("valueList() = %v, want %v", got, want)
	}

	if got := valueList([]string{"x", "y"}); !reflect.DeepEqual(got, []any{"x", "y"}) {
		t.Errorf("a binding's []string read as %v, want [x y]", got)
	}
	if got := valueList(model.NewArray()); len(got) != 0 {
		t.Errorf("an empty array read as %v, want nothing", got)
	}
}

// TestFilterByValue covers array_diff and array_intersect, which share this
// walk and differ in the verdict.
//
// Values compare as strings, PHP's documented rule for this family and
// array_diff([0], ["0"]) is empty. The count of arrays holding a value is what
// lets one pass answer both questions: with a third array passed, diff drops a
// value the second holds even when the third does not.
func TestFilterByValue(t *testing.T) {
	t.Parallel()
	list := setArray(int64(0), int64(1), int64(1), int64(2), int64(2), int64(3))

	for _, tc := range []struct {
		name   string
		others []any
		keep   bool
		want   []any
	}{
		{
			name:   "diff over two arrays keeps what neither holds",
			others: []any{setArray(int64(0), int64(2)), setArray(int64(0), int64(3))},
			want:   []any{int64(0), int64(1)},
		},
		{
			name:   "intersect over two arrays keeps what both hold",
			others: []any{setArray(int64(0), int64(2), int64(1), int64(3)), setArray(int64(0), int64(3))},
			keep:   true,
			want:   []any{int64(2), int64(3)},
		},
		{
			name: "diff with no other array keeps everything",
			want: []any{int64(0), int64(1), int64(1), int64(2), int64(2), int64(3)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := entries(filterByValue(list, tc.others, tc.keep))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("filterByValue() = %v, want %v", got, tc.want)
			}
		})
	}

	zero := setArray(int64(0), int64(0))
	if got := entries(filterByValue(zero, []any{setArray(int64(0), "0")}, false)); got != nil {
		t.Errorf("array_diff([0], [\"0\"]) = %v, want nothing: values compare as strings", got)
	}
}

// TestFilterByKey covers array_diff_key and array_intersect_key. Keys run
// through phpval.Key first and the integer 1 of an array meets the string "1" a
// binding's map[string]any carries as one key. Neither one reads a value or
// compares one.
func TestFilterByKey(t *testing.T) {
	t.Parallel()
	named := setArray(int64(1), "a", "b", "c")
	others := []any{map[string]any{"1": "z"}}

	kept := entries(filterByKey(named, others, true))
	if want := []any{int64(1), "a"}; !reflect.DeepEqual(kept, want) {
		t.Errorf("array_intersect_key() = %#v, want %#v", kept, want)
	}

	dropped := entries(filterByKey(named, others, false))
	if want := []any{"b", "c"}; !reflect.DeepEqual(dropped, want) {
		t.Errorf("array_diff_key() = %#v, want %#v", dropped, want)
	}
}

// TestWanted covers the rule turning "how many of the other arrays hold this"
// into a verdict. The two are not each other's negation: intersect wants every
// other array, diff wants none of them and they only coincide while there is
// one other array.
func TestWanted(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		count, others int
		keep          bool
		want          bool
	}{
		{count: 0, others: 1, keep: false, want: true},
		{count: 1, others: 1, keep: false, want: false},
		{count: 1, others: 1, keep: true, want: true},
		{count: 1, others: 2, keep: true, want: false},
		{count: 2, others: 2, keep: true, want: true},
		{count: 1, others: 2, keep: false, want: false},
	} {
		if got := wanted(tc.count, tc.others, tc.keep); got != tc.want {
			t.Errorf("wanted(%d, %d, %v) = %v, want %v", tc.count, tc.others, tc.keep, got, tc.want)
		}
	}
}

// TestEdgeKey covers array_key_first and array_key_last, which read an edge
// without an internal pointer. PHP's older spelling is reset() plus key(),
// which moves the pointer the read comes off; this runtime has none and an
// empty array answers null.
func TestEdgeKey(t *testing.T) {
	t.Parallel()
	sparse := setArray(int64(7), "a", "name", "b", int64(2), "c")

	if got := edgeKey(sparse, true); got != int64(7) {
		t.Errorf("array_key_first() = %#v, want int64(7)", got)
	}
	if got := edgeKey(sparse, false); got != int64(2) {
		t.Errorf("array_key_last() = %#v, want int64(2)", got)
	}
	if got := edgeKey(model.NewArray(), true); got != nil {
		t.Errorf("array_key_first() on an empty array = %#v, want nil", got)
	}
	if got := edgeKey(model.NewArray(), false); got != nil {
		t.Errorf("array_key_last() on an empty array = %#v, want nil", got)
	}
}
