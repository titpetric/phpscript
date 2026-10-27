package runner

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestDeepSize covers the walk memory_get_usage() and the limit checkpoints
// read. The walk counts a container reachable through two or more variables
// once, and ends on a self-referential one, because the visited set keys on
// pointer identity.
func TestDeepSize(t *testing.T) {
	t.Parallel()
	inner := model.NewArray()
	inner.Append("abcdefgh")

	outer := model.NewArray()
	outer.Append(inner)
	once := DeepSize(outer, make(visitedSet))

	outer.Append(inner)
	twice := DeepSize(outer, make(visitedSet))
	if twice <= once {
		t.Errorf("a second entry did not raise the size: %d, want more than %d", twice, once)
	}

	shared := model.NewArray()
	shared.Append(inner)
	shared.Append(inner)
	visited := make(visitedSet)
	if got := DeepSize(shared, visited); got >= DeepSize(inner, make(visitedSet))*2+64 {
		t.Errorf("the shared array was counted twice: %d", got)
	}

	cyclic := model.NewArray()
	cyclic.Append(cyclic)
	if got := DeepSize(cyclic, make(visitedSet)); got == 0 {
		t.Error("a self-referential array measured nothing")
	}
}

// TestDeepSizeContainers covers the container kinds a script owns. A nil
// container costs nothing, DeepSize charges an object for its property names
// as well as its values, and a host-owned Go value takes the shallow size, the
// ownership rule flatHost.SetEntry applies.
func TestDeepSizeContainers(t *testing.T) {
	t.Parallel()
	var nilArray *model.Array
	var nilObject *model.Object
	for _, tc := range []struct {
		name  string
		value any
	}{
		{"a nil array", nilArray},
		{"a nil object", nilObject},
		{"nil", nil},
	} {
		if got := DeepSize(tc.value, make(visitedSet)); got != 0 {
			t.Errorf("%s costs %d, want 0", tc.name, got)
		}
	}

	obj := model.NewObject(&model.Class{Name: "Row"})
	obj.SetProp("name", "ada")
	bare := model.NewObject(&model.Class{Name: "Row"})
	if DeepSize(obj, make(visitedSet)) <= DeepSize(bare, make(visitedSet)) {
		t.Error("a property added nothing to the measured size of an object")
	}

	nested := map[string]any{"rows": []any{"a", "b"}}
	if DeepSize(nested, make(visitedSet)) <= DeepSize(map[string]any{}, make(visitedSet)) {
		t.Error("the entries added nothing to the measured size of a map")
	}
}

// TestEstimateValueSize covers the shallow estimate charged to every binding's
// result. A scalar is its own width, a string is its header plus its bytes,
// and the walk that reads a deeper graph is DeepSize: this one undercounts
// nesting and is cheap enough to run per call.
func TestEstimateValueSize(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value any
		want  int64
	}{
		{"nil", nil, 0},
		{"a bool", true, 1},
		{"an int64", int64(1), 8},
		{"a float64", 1.5, 8},
		{"an int32", int32(1), 4},
		{"an int16", int16(1), 2},
		{"an int8", int8(1), 1},
		{"an empty string", "", 16},
		{"a string", "abcd", 20},
		{"bytes", []byte("abcd"), 28},
	} {
		if got := EstimateValueSize(tc.value); got != tc.want {
			t.Errorf("%s: EstimateValueSize() = %d, want %d", tc.name, got, tc.want)
		}
	}

	deep := model.NewArray()
	leaf := model.NewArray()
	leaf.Append("abcdefghijklmnop")
	deep.Append(leaf)
	if EstimateValueSize(deep) >= DeepSize(deep, make(visitedSet)) {
		t.Error("the shallow estimate is not below the walk on a nested array")
	}
}

// TestShallowElementSize covers what one entry of a container costs while the
// shallow estimate walks it. A nested container costs its header and nothing
// below it and the estimate stays one level deep whatever it holds.
func TestShallowElementSize(t *testing.T) {
	t.Parallel()
	filled := model.NewArray()
	filled.Append("abcdefgh")

	for _, tc := range []struct {
		name  string
		value any
		want  int64
	}{
		{"nil", nil, 8},
		{"a bool", false, 1},
		{"an int64", int64(1), 8},
		{"a string", "abcd", 20},
		{"bytes", []byte("abcd"), 28},
		{"an array", filled, 64},
		{"an empty array", model.NewArray(), 64},
		{"an object", model.NewObject(nil), 64},
		{"a host value", &Scope{}, 16},
	} {
		if got := shallowElementSize(tc.value); got != tc.want {
			t.Errorf("%s: shallowElementSize() = %d, want %d", tc.name, got, tc.want)
		}
	}
}
