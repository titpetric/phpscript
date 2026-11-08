package modelutil_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// listArray is the array every test below reads, in the one shape that keeps
// its order: a PHP list.
func listArray(items ...any) *model.Array {
	a := model.NewArraySize(len(items))
	a.AppendAll(items)
	return a
}

// TestSeq covers the range-over-func form of RangeValues: a `break` in the
// loop body is the `return false` the closure form writes.
func TestSeq(t *testing.T) {
	t.Parallel()
	var keys, values []any
	for key, val := range modelutil.Seq(listArray("a", "b", "c")) {
		keys = append(keys, key)
		values = append(values, val)
	}
	if want := []any{int64(0), int64(1), int64(2)}; !reflect.DeepEqual(keys, want) {
		t.Errorf("keys = %v, want %v", keys, want)
	}
	if want := []any{"a", "b", "c"}; !reflect.DeepEqual(values, want) {
		t.Errorf("values = %v, want %v", values, want)
	}

	var seen int
	for range modelutil.Seq([]string{"a", "b", "c"}) {
		seen++
		break
	}
	if seen != 1 {
		t.Errorf("a break visited %d entries, want 1", seen)
	}
}

// TestKeys covers the keys of every shape a collection arrives in, the loop the
// array bindings each wrote by hand.
func TestKeys(t *testing.T) {
	t.Parallel()
	if got, want := modelutil.Keys(listArray("a", "b")), []any{int64(0), int64(1)}; !reflect.DeepEqual(got, want) {
		t.Errorf("Keys(array) = %v, want %v", got, want)
	}
	if got, want := modelutil.Keys([]string{"a", "b"}), []any{int64(0), int64(1)}; !reflect.DeepEqual(got, want) {
		t.Errorf("Keys([]string) = %v, want %v", got, want)
	}
	if got, want := modelutil.Keys(map[string]any{"k": 1}), []any{"k"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Keys(map) = %v, want %v", got, want)
	}
	if got := modelutil.Keys("a string"); len(got) != 0 {
		t.Errorf("Keys(string) = %v, want nothing for a value that is no collection", got)
	}
}

// TestValues covers the same over the values.
func TestValues(t *testing.T) {
	t.Parallel()
	if got, want := modelutil.Values(listArray("a", "b")), []any{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Values(array) = %v, want %v", got, want)
	}
	if got, want := modelutil.Values([]any{1, 2}), []any{1, 2}; !reflect.DeepEqual(got, want) {
		t.Errorf("Values([]any) = %v, want %v", got, want)
	}
	if got := modelutil.Values(nil); len(got) != 0 {
		t.Errorf("Values(nil) = %v, want nothing", got)
	}
}

// TestTyped covers the typed read a binding declaring its element type makes,
// and the error naming the key that holds something else.
func TestTyped(t *testing.T) {
	t.Parallel()
	got, err := modelutil.Typed[string](listArray("a", "b"))
	if err != nil {
		t.Fatalf("Typed[string]() error = %v", err)
	}
	if want := []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Typed[string]() = %v, want %v", got, want)
	}

	if _, err := modelutil.Typed[string](listArray("a", int64(2))); err == nil {
		t.Error("Typed[string]() = nil error for an int64 entry")
	} else if !strings.Contains(err.Error(), "entry 1") {
		t.Errorf("Typed[string]() = %v, want it to name entry 1", err)
	}

	empty, err := modelutil.Typed[int64](nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("Typed[int64](nil) = %v, %v; want an empty slice and no error", empty, err)
	}
}

// TestMap covers the string-keyed read a template makes, where an integer key
// reaches it as its decimal spelling.
func TestMap(t *testing.T) {
	t.Parallel()
	got := modelutil.Map(listArray("a", "b"))
	if want := map[string]any{"0": "a", "1": "b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Map(array) = %v, want %v", got, want)
	}
	if got := modelutil.Map(map[string]any{"k": 1}); !reflect.DeepEqual(got, map[string]any{"k": 1}) {
		t.Errorf("Map(map) = %v", got)
	}
	if got := modelutil.Map(nil); len(got) != 0 {
		t.Errorf("Map(nil) = %v, want nothing", got)
	}
}
