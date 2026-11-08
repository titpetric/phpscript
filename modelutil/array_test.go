package modelutil_test

import (
	"reflect"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// TestToArray covers the conversion into the one shape PHP mutates. An
// existing array passes through untouched, a Go slice becomes a list and a
// string-keyed map becomes a keyed array.
//
// The copy is what the test is for: a script that appends to the result must
// not write through to the slice a binding returned.
func TestToArray(t *testing.T) {
	t.Parallel()

	existing := model.NewArray()
	existing.Append("a")
	if got := modelutil.ToArray(existing); got != existing {
		t.Error("ToArray copied an *model.Array it was handed")
	}

	src := []any{"a", "b", "c"}
	list := modelutil.ToArray(src)
	want := []any{int64(0), int64(1), int64(2)}
	if got := list.Keys(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Keys() = %v, want %v", got, want)
	}
	if got := list.NextIndex(); got != 3 {
		t.Fatalf("NextIndex() = %d, want 3", got)
	}
	list.Append("d")
	if len(src) != 3 {
		t.Fatal("ToArray must copy: appending wrote through to the source slice")
	}

	keyed := modelutil.ToArray(map[string]any{"k": "v"})
	if v, ok := keyed.Get("k"); !ok || v != "v" {
		t.Fatalf(`Get("k") = %v, %v`, v, ok)
	}

	typed := modelutil.ToArray([]string{"x", "y"})
	if v, ok := typed.Get(int64(1)); !ok || v != "y" {
		t.Fatalf("Get(1) = %v, %v; want y, true for a []string", v, ok)
	}

	if got := modelutil.ToArray(nil).Len(); got != 0 {
		t.Fatalf("ToArray(nil).Len() = %d, want 0", got)
	}
}
