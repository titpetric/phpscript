package model

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"
)

// legacyArray is the map-backed *Array as it was before list mode: every array
// allocated a map[any]any and a key slice up front. The differential test below
// replays operation sequences against both implementations and requires
// identical observable behaviour; the benchmarks measure what list mode saves.
// Following docs/allocation-performance.md ("keep the old implementation as a
// second binding so the benchmark measures the change instead of asserting it").
type legacyArray struct {
	keys   []any
	values map[any]any
	nextID int64
}

func newLegacyArray() *legacyArray { return &legacyArray{values: map[any]any{}} }

func newLegacyArraySize(n int) *legacyArray {
	if n <= 0 {
		return newLegacyArray()
	}
	return &legacyArray{keys: make([]any, 0, n), values: make(map[any]any, n)}
}

func (a *legacyArray) Set(key, val any) {
	if _, ok := a.values[key]; !ok {
		a.keys = append(a.keys, key)
	}
	a.values[key] = val
	if i, ok := key.(int64); ok && i >= a.nextID {
		a.nextID = i + 1
	}
}

func (a *legacyArray) Append(val any) { a.Set(a.nextID, val) }
func (a *legacyArray) Get(key any) (any, bool) {
	v, ok := a.values[key]
	return v, ok
}
func (a *legacyArray) Len() int    { return len(a.keys) }
func (a *legacyArray) Keys() []any { return a.keys }
func (a *legacyArray) Range(fn func(key, val any) bool) {
	for _, k := range a.keys {
		if !fn(k, a.values[k]) {
			return
		}
	}
}
func (a *legacyArray) Map() map[string]any {
	result := make(map[string]any, len(a.keys))
	for _, key := range a.keys {
		result[fmt.Sprint(key)] = a.values[key]
	}
	return result
}
func (a *legacyArray) Clear() {
	a.keys = nil
	a.values = map[any]any{}
	a.nextID = 0
}

// entries collects an array's key/value pairs in iteration order.
func entries(a *Array) [][2]any {
	var out [][2]any
	a.Range(func(k, v any) bool {
		out = append(out, [2]any{k, v})
		return true
	})
	return out
}

func legacyEntries(a *legacyArray) [][2]any {
	var out [][2]any
	a.Range(func(k, v any) bool {
		out = append(out, [2]any{k, v})
		return true
	})
	return out
}

func TestArrayListModeAppend(t *testing.T) {
	a := NewArray()
	if !a.isList() {
		t.Fatal("a fresh array should start in list mode")
	}
	for i := range 5 {
		a.Append("v" + strconv.Itoa(i))
	}
	if !a.isList() {
		t.Fatal("appends alone must not promote the array")
	}
	if a.values != nil || a.keys != nil {
		t.Fatalf("list mode must not allocate a map or key slice: keys=%v values=%v", a.keys, a.values)
	}
	if got := a.Len(); got != 5 {
		t.Fatalf("Len() = %d, want 5", got)
	}
	if got, want := a.Keys(), []any{int64(0), int64(1), int64(2), int64(3), int64(4)}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Keys() = %v, want %v", got, want)
	}
	for i := range 5 {
		v, ok := a.Get(int64(i))
		if !ok || v != "v"+strconv.Itoa(i) {
			t.Fatalf("Get(%d) = %v, %v", i, v, ok)
		}
	}
	if _, ok := a.Get(int64(5)); ok {
		t.Fatal("Get(5) on a 5-element list should miss")
	}
	if _, ok := a.Get(int64(-1)); ok {
		t.Fatal("Get(-1) should miss, not panic or wrap")
	}
	want := [][2]any{{int64(0), "v0"}, {int64(1), "v1"}, {int64(2), "v2"}, {int64(3), "v3"}, {int64(4), "v4"}}
	if got := entries(a); !reflect.DeepEqual(got, want) {
		t.Fatalf("Range() = %v, want %v", got, want)
	}
}

func TestArrayNewArraySizePresizesListOnly(t *testing.T) {
	a := NewArraySize(8)
	if !a.isList() {
		t.Fatal("NewArraySize should return a list-mode array")
	}
	if a.values != nil || a.keys != nil {
		t.Fatal("NewArraySize must not allocate a map or key slice")
	}
	if cap(a.list) != 8 {
		t.Fatalf("cap(list) = %d, want 8", cap(a.list))
	}
	if a.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", a.Len())
	}
	if NewArraySize(0).list != nil {
		t.Fatal("NewArraySize(0) should allocate nothing")
	}
}

// TestArrayMixedLiteral is PHP's array("a", "b", "k" => "c", "d"), whose keys
// are 0, 1, "k", 2 in that order: the string key does not advance the append
// index. A list-mode implementation that dropped insertion order or restarted
// the index at the string key would fail here.
func TestArrayMixedLiteral(t *testing.T) {
	a := NewArray()
	a.Append("a")
	a.Append("b")
	a.Set("k", "c")
	a.Append("d")

	if a.isList() {
		t.Fatal("a string key must promote the array to map mode")
	}
	want := [][2]any{{int64(0), "a"}, {int64(1), "b"}, {"k", "c"}, {int64(2), "d"}}
	if got := entries(a); !reflect.DeepEqual(got, want) {
		t.Fatalf("Range() = %v, want %v", got, want)
	}
	if got := a.Len(); got != 4 {
		t.Fatalf("Len() = %d, want 4", got)
	}
	if got, want := a.Keys(), []any{int64(0), int64(1), "k", int64(2)}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Keys() = %v, want %v", got, want)
	}
	if v, ok := a.Get("k"); !ok || v != "c" {
		t.Fatalf(`Get("k") = %v, %v`, v, ok)
	}
	if v, ok := a.Get(int64(1)); !ok || v != "b" {
		t.Fatalf("Get(1) = %v, %v; the promoted elements must survive", v, ok)
	}
}

// TestArraySparseIntKeys covers PHP's array(5 => "a", "b"): the next append
// lands at 6, and a later low key does not move.
func TestArraySparseIntKeys(t *testing.T) {
	a := NewArray()
	a.Set(int64(5), "a")
	if a.isList() {
		t.Fatal("a sparse int key must promote")
	}
	a.Append("b")
	a.Set(int64(0), "c")

	want := [][2]any{{int64(5), "a"}, {int64(6), "b"}, {int64(0), "c"}}
	if got := entries(a); !reflect.DeepEqual(got, want) {
		t.Fatalf("Range() = %v, want %v", got, want)
	}
	if a.nextID != 7 {
		t.Fatalf("nextID = %d, want 7 (a low key must not rewind the append index)", a.nextID)
	}
}

// TestArrayOutOfOrderSetPromotes: writing key 1 before key 0 is not a list.
// Its iteration order is 1 then 0, which a naive "append into the slice"
// implementation would silently reorder.
func TestArrayOutOfOrderSetPromotes(t *testing.T) {
	a := NewArray()
	a.Set(int64(1), "b")
	if a.isList() {
		t.Fatal("Set(1) on an empty array must promote")
	}
	a.Set(int64(0), "a")

	want := [][2]any{{int64(1), "b"}, {int64(0), "a"}}
	if got := entries(a); !reflect.DeepEqual(got, want) {
		t.Fatalf("Range() = %v, want %v", got, want)
	}
	if a.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", a.Len())
	}
}

// TestArrayListModeOverwrite: an in-range int64 key overwrites in place, keeps
// the array in list mode, and does not change Len or the append index.
func TestArrayListModeOverwrite(t *testing.T) {
	a := NewArray()
	a.Append("a")
	a.Append("b")
	a.Append("c")

	a.Set(int64(1), "B")
	if !a.isList() {
		t.Fatal("overwriting an existing index must not promote")
	}
	if a.Len() != 3 {
		t.Fatalf("Len() = %d, want 3 (an overwrite is not an insert)", a.Len())
	}
	if a.nextID != 3 {
		t.Fatalf("nextID = %d, want 3", a.nextID)
	}
	a.Set(int64(3), "d") // exactly at the end: still a list
	if !a.isList() {
		t.Fatal("setting the key one past the end is an append, not a promotion")
	}
	want := [][2]any{{int64(0), "a"}, {int64(1), "B"}, {int64(2), "c"}, {int64(3), "d"}}
	if got := entries(a); !reflect.DeepEqual(got, want) {
		t.Fatalf("Range() = %v, want %v", got, want)
	}
}

// TestArrayNegativeKey: PHP 7 semantics as this Array models them — a negative
// key does not lower the append index (see the `i >= a.nextID` guard).
func TestArrayNegativeKey(t *testing.T) {
	a := NewArray()
	a.Set(int64(-3), "x")
	if a.isList() {
		t.Fatal("a negative key must promote")
	}
	a.Append("y")
	want := [][2]any{{int64(-3), "x"}, {int64(0), "y"}}
	if got := entries(a); !reflect.DeepEqual(got, want) {
		t.Fatalf("Range() = %v, want %v", got, want)
	}
}

// TestArrayKeysAreOpaque locks in that the Array itself does not normalise
// keys: "0", int(0) and int64(0) are three distinct keys. Normalisation is the
// runner's job (runner.normalizeKey), so list mode must only recognise int64.
func TestArrayKeysAreOpaque(t *testing.T) {
	a := NewArray()
	a.Set(int(0), "int")
	if a.isList() {
		t.Fatal("an int (not int64) key must promote: it is a different map key")
	}
	a.Set("0", "string")
	a.Set(int64(0), "int64")

	if a.Len() != 3 {
		t.Fatalf("Len() = %d, want 3 distinct keys", a.Len())
	}
	if a.nextID != 1 {
		t.Fatalf("nextID = %d, want 1 (only an int64 key advances it)", a.nextID)
	}
	for key, want := range map[any]string{int(0): "int", "0": "string", int64(0): "int64"} {
		if v, ok := a.Get(key); !ok || v != want {
			t.Fatalf("Get(%#v) = %v, %v; want %q", key, v, ok, want)
		}
	}

	// The same distinction has to hold in list mode, where only int64 hits.
	b := NewArray()
	b.Append("v")
	if _, ok := b.Get(int(0)); ok {
		t.Fatal("Get(int(0)) must miss in list mode, matching the map-backed array")
	}
	if _, ok := b.Get("0"); ok {
		t.Fatal(`Get("0") must miss in list mode`)
	}
	if _, ok := b.Get(int64(0)); !ok {
		t.Fatal("Get(int64(0)) must hit in list mode")
	}
}

func TestArrayFloatKeyPromotes(t *testing.T) {
	a := NewArray()
	a.Set(float64(0), "f")
	if a.isList() {
		t.Fatal("a float key must promote")
	}
	a.Append("v")
	want := [][2]any{{float64(0), "f"}, {int64(0), "v"}}
	if got := entries(a); !reflect.DeepEqual(got, want) {
		t.Fatalf("Range() = %v, want %v", got, want)
	}
}

// TestArrayClearReturnsToListMode covers array_splice/usort, which Clear an
// array and rebuild it with Append.
func TestArrayClearReturnsToListMode(t *testing.T) {
	a := NewArray()
	a.Set("k", "v")
	a.Append("x")
	if a.isList() {
		t.Fatal("string key should have promoted")
	}

	a.Clear()
	if !a.isList() {
		t.Fatal("Clear must return the array to list mode")
	}
	if a.Len() != 0 || a.nextID != 0 {
		t.Fatalf("after Clear: Len=%d nextID=%d, want 0/0", a.Len(), a.nextID)
	}
	if _, ok := a.Get("k"); ok {
		t.Fatal("Clear must drop the old keys")
	}
	if _, ok := a.Get(int64(0)); ok {
		t.Fatal("Clear must drop the old values")
	}
	a.Append("a")
	a.Append("b")
	want := [][2]any{{int64(0), "a"}, {int64(1), "b"}}
	if got := entries(a); !reflect.DeepEqual(got, want) {
		t.Fatalf("Range() after Clear = %v, want %v", got, want)
	}
}

func TestArrayRangeStopsEarly(t *testing.T) {
	for _, mode := range []string{"list", "map"} {
		a := NewArray()
		if mode == "map" {
			a.Set("k", "promoted")
		}
		for i := range 4 {
			a.Append(i)
		}
		var seen int
		a.Range(func(_, _ any) bool {
			seen++
			return seen < 2
		})
		if seen != 2 {
			t.Fatalf("%s mode: Range visited %d entries after the callback stopped, want 2", mode, seen)
		}
	}
}

// TestArrayPromoteDuringRange is the hazard list mode introduces: a script may
// add a string key from inside its own foreach. Promotion must not scramble the
// slice the in-flight Range is walking.
func TestArrayPromoteDuringRange(t *testing.T) {
	a := NewArray()
	a.Append("a")
	a.Append("b")
	a.Append("c")

	var got [][2]any
	a.Range(func(k, v any) bool {
		got = append(got, [2]any{k, v})
		if k == int64(0) {
			a.Set("added", "z")
		}
		return true
	})
	want := [][2]any{{int64(0), "a"}, {int64(1), "b"}, {int64(2), "c"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Range during promotion = %v, want %v", got, want)
	}
	if v, ok := a.Get("added"); !ok || v != "z" {
		t.Fatalf(`Get("added") = %v, %v`, v, ok)
	}
	if a.Len() != 4 {
		t.Fatalf("Len() = %d, want 4", a.Len())
	}
}

func TestArrayMapStringKeys(t *testing.T) {
	list := NewArray()
	list.Append("a")
	list.Append("b")
	if got, want := list.Map(), map[string]any{"0": "a", "1": "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("list Map() = %v, want %v", got, want)
	}

	mixed := NewArray()
	mixed.Append("a")
	mixed.Set("k", "c")
	if got, want := mixed.Map(), map[string]any{"0": "a", "k": "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("map Map() = %v, want %v", got, want)
	}
}

func TestArrayZeroValueIsUsable(t *testing.T) {
	var a Array
	a.Append("x")
	a.Set("k", "v")
	if a.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", a.Len())
	}
	if v, ok := a.Get(int64(0)); !ok || v != "x" {
		t.Fatalf("Get(0) = %v, %v", v, ok)
	}
}

func TestToArrayListFastPath(t *testing.T) {
	src := []any{"a", "b", "c"}
	a := ToArray(src)
	if !a.isList() {
		t.Fatal("ToArray of a []any should produce a list-mode array")
	}
	if a.nextID != 3 {
		t.Fatalf("nextID = %d, want 3", a.nextID)
	}
	want := [][2]any{{int64(0), "a"}, {int64(1), "b"}, {int64(2), "c"}}
	if got := entries(a); !reflect.DeepEqual(got, want) {
		t.Fatalf("Range() = %v, want %v", got, want)
	}
	a.Append("d")
	if len(src) != 3 {
		t.Fatal("ToArray must copy: appending to the Array wrote through to the source slice")
	}
	a.Set(int64(0), "A")
	if src[0] != "a" {
		t.Fatal("ToArray must copy: Set wrote through to the source slice")
	}

	m := ToArray(map[string]any{"k": "v"})
	if m.isList() {
		t.Fatal("ToArray of a string-keyed map must be map mode")
	}
	if v, ok := m.Get("k"); !ok || v != "v" {
		t.Fatalf(`Get("k") = %v, %v`, v, ok)
	}
}

func TestRangeValuesAndLenValuesOnBothModes(t *testing.T) {
	list := NewArray()
	list.Append("a")
	list.Append("b")

	mapped := NewArray()
	mapped.Append("a")
	mapped.Set("k", "b")

	for name, a := range map[string]*Array{"list": list, "map": mapped} {
		n, ok := LenValues(a)
		if !ok || n != 2 {
			t.Fatalf("%s: LenValues = %d, %v; want 2, true", name, n, ok)
		}
		if !IsCollection(a) {
			t.Fatalf("%s: IsCollection = false", name)
		}
		var count int
		RangeValues(a, func(_, _ any) bool { count++; return true })
		if count != 2 {
			t.Fatalf("%s: RangeValues visited %d entries, want 2", name, count)
		}
	}
}

// op is one step of the differential test below.
type op struct {
	kind string // "append", "set", "clear"
	key  any
	val  any
}

// TestArrayMatchesLegacyImplementation replays operation sequences against the
// list-mode Array and against the map-backed implementation it replaces, and
// requires identical keys, values, iteration order, Len and append index. Any
// list-mode shortcut that gets PHP's key rules wrong shows up here.
func TestArrayMatchesLegacyImplementation(t *testing.T) {
	sequences := map[string][]op{
		"pure list": {
			{kind: "append", val: "a"}, {kind: "append", val: "b"}, {kind: "append", val: "c"},
		},
		"explicit dense int keys": {
			{kind: "set", key: int64(0), val: "a"}, {kind: "set", key: int64(1), val: "b"},
		},
		"literal with string key in the middle": {
			{kind: "append", val: "a"}, {kind: "set", key: "k", val: "c"}, {kind: "append", val: "d"},
		},
		"string key first": {
			{kind: "set", key: "k", val: "v"}, {kind: "append", val: "a"}, {kind: "append", val: "b"},
		},
		"sparse": {
			{kind: "set", key: int64(5), val: "a"}, {kind: "append", val: "b"},
			{kind: "set", key: int64(2), val: "c"},
		},
		"reverse order": {
			{kind: "set", key: int64(2), val: "c"}, {kind: "set", key: int64(1), val: "b"},
			{kind: "set", key: int64(0), val: "a"},
		},
		"overwrite in place": {
			{kind: "append", val: "a"}, {kind: "append", val: "b"},
			{kind: "set", key: int64(0), val: "A"}, {kind: "append", val: "c"},
		},
		"duplicate key": {
			{kind: "set", key: int64(0), val: "a"}, {kind: "set", key: int64(0), val: "b"},
		},
		"negative key": {
			{kind: "set", key: int64(-2), val: "n"}, {kind: "append", val: "a"},
			{kind: "set", key: int64(-2), val: "N"},
		},
		"non-int64 numeric keys": {
			{kind: "set", key: int(1), val: "int"}, {kind: "set", key: "1", val: "string"},
			{kind: "append", val: "a"},
		},
		"clear then rebuild": {
			{kind: "append", val: "a"}, {kind: "set", key: "k", val: "v"},
			{kind: "clear"}, {kind: "append", val: "x"}, {kind: "append", val: "y"},
		},
		"clear a list then use string keys": {
			{kind: "append", val: "a"}, {kind: "clear"},
			{kind: "set", key: "k", val: "v"}, {kind: "append", val: "b"},
		},
		"append past 255 boxes": {
			{kind: "set", key: int64(300), val: "a"}, {kind: "append", val: "b"},
		},
		"nil value": {
			{kind: "append", val: nil}, {kind: "set", key: "k", val: nil}, {kind: "append", val: nil},
		},
	}

	probes := []any{int64(-2), int64(0), int64(1), int64(2), int64(5), int64(300), int64(301),
		int(0), int(1), "0", "1", "k", "missing", float64(0)}

	for name, ops := range sequences {
		t.Run(name, func(t *testing.T) {
			got, want := NewArray(), newLegacyArray()
			for _, o := range ops {
				switch o.kind {
				case "append":
					got.Append(o.val)
					want.Append(o.val)
				case "set":
					got.Set(o.key, o.val)
					want.Set(o.key, o.val)
				case "clear":
					got.Clear()
					want.Clear()
				default:
					t.Fatalf("unknown op %q", o.kind)
				}
			}

			if got.Len() != want.Len() {
				t.Fatalf("Len() = %d, want %d", got.Len(), want.Len())
			}
			if got.nextID != want.nextID {
				t.Fatalf("nextID = %d, want %d", got.nextID, want.nextID)
			}
			if g, w := entries(got), legacyEntries(want); !reflect.DeepEqual(g, w) {
				t.Fatalf("Range() = %v, want %v", g, w)
			}
			if g, w := got.Keys(), want.Keys(); !keysEqual(g, w) {
				t.Fatalf("Keys() = %v, want %v", g, w)
			}
			if g, w := got.Map(), want.Map(); !reflect.DeepEqual(g, w) {
				t.Fatalf("Map() = %v, want %v", g, w)
			}
			for _, probe := range probes {
				gv, gok := got.Get(probe)
				wv, wok := want.Get(probe)
				if gok != wok || !reflect.DeepEqual(gv, wv) {
					t.Fatalf("Get(%#v) = %v, %v; want %v, %v", probe, gv, gok, wv, wok)
				}
			}
		})
	}
}

// keysEqual compares key slices, treating nil and empty as the same (list mode
// materialises its keys, so an empty array yields an empty non-nil slice).
func keysEqual(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !reflect.DeepEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// Benchmarks. Run with:
//
//	go test ./model/ -run XXX -bench BenchmarkArray -benchmem
//
// The Legacy variants build the same array through the pre-list-mode
// implementation, so the delta is the change itself.

func benchValues(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "value-" + strconv.Itoa(i)
	}
	return out
}

func benchArrayAppend(b *testing.B, n int, presize bool) {
	values := benchValues(n)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var a *Array
		if presize {
			a = NewArraySize(n)
		} else {
			a = NewArray()
		}
		for _, v := range values {
			a.Append(v)
		}
		if a.Len() != n {
			b.Fatal("short array")
		}
	}
}

func benchLegacyAppend(b *testing.B, n int, presize bool) {
	values := benchValues(n)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var a *legacyArray
		if presize {
			a = newLegacyArraySize(n)
		} else {
			a = newLegacyArray()
		}
		for _, v := range values {
			a.Append(v)
		}
		if a.Len() != n {
			b.Fatal("short array")
		}
	}
}

func BenchmarkArrayAppend5(b *testing.B)             { benchArrayAppend(b, 5, false) }
func BenchmarkArrayAppend5Legacy(b *testing.B)       { benchLegacyAppend(b, 5, false) }
func BenchmarkArrayAppend5Sized(b *testing.B)        { benchArrayAppend(b, 5, true) }
func BenchmarkArrayAppend5SizedLegacy(b *testing.B)  { benchLegacyAppend(b, 5, true) }
func BenchmarkArrayAppend50(b *testing.B)            { benchArrayAppend(b, 50, false) }
func BenchmarkArrayAppend50Legacy(b *testing.B)      { benchLegacyAppend(b, 50, false) }
func BenchmarkArrayAppend50Sized(b *testing.B)       { benchArrayAppend(b, 50, true) }
func BenchmarkArrayAppend50SizedLegacy(b *testing.B) { benchLegacyAppend(b, 50, true) }

// The Ints variants store values in 0..255, which the runtime boxes for free
// (staticuint64s), so what is left is the array's own structural cost.
func BenchmarkArrayAppend5Ints(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		a := NewArraySize(5)
		for i := range 5 {
			a.Append(int64(i))
		}
	}
}

func BenchmarkArrayAppend5IntsLegacy(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		a := newLegacyArraySize(5)
		for i := range 5 {
			a.Append(int64(i))
		}
	}
}

// The string-keyed build is the case list mode must not make worse: the array
// promotes on the first key, so it should cost what the map-backed one did.
func BenchmarkArrayStringKeys5(b *testing.B) {
	keys := benchValues(5)
	b.ReportAllocs()
	for range b.N {
		a := NewArraySize(5)
		for i, k := range keys {
			a.Set(k, i)
		}
	}
}

func BenchmarkArrayStringKeys5Legacy(b *testing.B) {
	keys := benchValues(5)
	b.ReportAllocs()
	for range b.N {
		a := newLegacyArraySize(5)
		for i, k := range keys {
			a.Set(k, i)
		}
	}
}

// A five-element list literal with one string key: the mixed shape that pays
// for promotion.
func BenchmarkArrayMixedLiteral(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		a := NewArray()
		a.Append("a")
		a.Append("b")
		a.Set("k", "c")
		a.Append("d")
		a.Append("e")
	}
}

func BenchmarkArrayMixedLiteralLegacy(b *testing.B) {
	b.ReportAllocs()
	for range b.N {
		a := newLegacyArray()
		a.Append("a")
		a.Append("b")
		a.Set("k", "c")
		a.Append("d")
		a.Append("e")
	}
}

func BenchmarkArrayRange50(b *testing.B) {
	a := NewArraySize(50)
	for _, v := range benchValues(50) {
		a.Append(v)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var n int
		a.Range(func(any, any) bool { n++; return true })
	}
}

func BenchmarkArrayRange50Legacy(b *testing.B) {
	a := newLegacyArraySize(50)
	for _, v := range benchValues(50) {
		a.Append(v)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var n int
		a.Range(func(any, any) bool { n++; return true })
	}
}
