package core

import (
	"reflect"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// cases assert it came back list-shaped, i.e. dense int64 keys in order.
func TestPHPArrayMergeShapes(t *testing.T) {
	assoc := model.NewArray()
	assoc.Set("a", 1)
	assoc.Set("b", 2)

	list := model.NewArray()
	list.Append("x")
	list.Append("y")

	sparse := model.NewArray()
	sparse.Set(int64(3), "three")

	cases := []struct {
		name     string
		args     []any
		wantList []any
		wantMap  map[string]any
		wantKeys []any
	}{
		{
			name:     "two []string lists",
			args:     []any{[]string{"a", "b"}, []string{"c"}},
			wantList: []any{"a", "b", "c"},
		},
		{
			name:     "[]any and list Array",
			args:     []any{[]any{"q"}, list},
			wantList: []any{"q", "x", "y"},
		},
		{
			name:     "no arguments",
			args:     nil,
			wantList: []any{},
		},
		{
			name:     "nil argument",
			args:     []any{nil, []string{"a"}},
			wantList: []any{"a"},
		},
		{
			name:     "string keys keep the Array",
			args:     []any{assoc, []string{"tail"}},
			wantMap:  map[string]any{"a": 1, "b": 2},
			wantKeys: []any{"a", "b", int64(0)},
		},
		{
			name:     "map argument keeps the Array",
			args:     []any{map[string]any{"k": "v"}},
			wantMap:  map[string]any{"k": "v"},
			wantKeys: []any{"k"},
		},
		{
			name:     "sparse int keys keep the Array",
			args:     []any{sparse, []string{"tail"}},
			wantKeys: []any{int64(0), int64(1)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := phpArrayMerge(tc.args...)
			arr, ok := got.(*model.Array)
			if !ok {
				t.Fatalf("array_merge returned %T, want *model.Array", got)
			}
			if tc.wantList != nil {
				if !arrayIsList(arr) {
					t.Fatalf("array_merge keys = %v, want the dense 0..n-1 list", arr.Keys())
				}
				var vals []any
				arr.Range(func(_, v any) bool { vals = append(vals, v); return true })
				if len(vals) != len(tc.wantList) {
					t.Fatalf("array_merge = %v, want %v", vals, tc.wantList)
				}
				for i := range vals {
					if vals[i] != tc.wantList[i] {
						t.Fatalf("array_merge = %v, want %v", vals, tc.wantList)
					}
				}
				return
			}
			for k, want := range tc.wantMap {
				if v, ok := arr.Get(k); !ok || v != want {
					t.Errorf("array_merge[%q] = %v (%v), want %v", k, v, ok, want)
				}
			}
			if tc.wantKeys != nil {
				keys := arr.Keys()
				if len(keys) != len(tc.wantKeys) {
					t.Fatalf("array_merge keys = %v, want %v", keys, tc.wantKeys)
				}
				for i := range keys {
					if keys[i] != tc.wantKeys[i] {
						t.Fatalf("array_merge keys = %v, want %v", keys, tc.wantKeys)
					}
				}
			}
		})
	}
}

// TestPHPArrayMergeCopiesInputs asserts array_merge produces a new array, as
// PHP does: writing to the result must not reach back into an argument.
func TestPHPArrayMergeCopiesInputs(t *testing.T) {
	src := []any{"a", "b"}
	out, ok := phpArrayMerge(src).(*model.Array)
	if !ok {
		t.Fatalf("array_merge returned %T, want *model.Array", out)
	}
	out.Set(int64(0), "changed")
	if src[0] != "a" {
		t.Errorf("array_merge aliased its argument: src = %v", src)
	}
}

// TestPHPArrayMergeResultIsAppendable is the reason array_merge does not
// return a []any for the all-lists case: a Go slice cannot grow through the
// interface value holding it, so `$x = array_merge($a, $b); $x[] = "z"` would
// be an error. Rule 4 of docs/allocation-performance.md.
func TestPHPArrayMergeResultIsAppendable(t *testing.T) {
	out, ok := phpArrayMerge([]string{"a"}, []string{"b"}).(*model.Array)
	if !ok {
		t.Fatalf("array_merge returned a non-appendable shape")
	}
	out.Append("z")
	if got := out.Len(); got != 3 {
		t.Fatalf("Len after append = %d, want 3", got)
	}
	if v, ok := out.Get(int64(2)); !ok || v != "z" {
		t.Errorf("appended element = %v (%v), want z", v, ok)
	}
}

// TestArrayMutatorsRejectNonArrays covers the one case a .phpt fixture cannot:
// the four mutators resize their argument, so they require a *model.Array and
// must say so rather than silently mutating a copy the script never sees. PHP
// has no way to hand them a Go slice, so only a Go test can reach this path.
func TestArrayMutatorsRejectNonArrays(t *testing.T) {
	// The shapes a binding hands back in place of a *model.Array.
	inputs := []any{nil, (*model.Array)(nil), []any{"a"}, []string{"a"}, "str"}

	mutators := []struct {
		name string
		call func(any) error
	}{
		{"array_shift", func(v any) error { _, err := phpArrayShift(v); return err }},
		{"array_pop", func(v any) error { _, err := phpArrayPop(v); return err }},
		{"array_unshift", func(v any) error { _, err := phpArrayUnshift(v, "x"); return err }},
		{"array_push", func(v any) error { _, err := phpArrayPush(v, "x"); return err }},
	}

	for _, m := range mutators {
		t.Run(m.name, func(t *testing.T) {
			for _, in := range inputs {
				err := m.call(in)
				if err == nil {
					t.Errorf("%s(%T) = nil error, want a rejection", m.name, in)
					continue
				}
				if !strings.HasPrefix(err.Error(), m.name+":") {
					t.Errorf("%s(%T) error = %q, want it named by the function", m.name, in, err)
				}
			}
			// The same call on a real array must still work.
			a := model.NewArray()
			a.Append("a")
			if err := m.call(a); err != nil {
				t.Errorf("%s(*model.Array) = %v, want no error", m.name, err)
			}
		})
	}
}

// TestArrayFindStrict pins the comparison in_array and array_search share: the
// loose path is phpval.Compare, so PHP 8's "a non-numeric string does not equal
// 0" holds, and the strict path compares the Go type first.
func TestArrayFindStrict(t *testing.T) {
	cases := []struct {
		name    string
		needle  any
		hay     any
		strict  bool
		wantKey any
		wantOK  bool
	}{
		{name: "numeric string matches int", needle: "1", hay: []any{int64(1)}, wantKey: int64(0), wantOK: true},
		{name: "numeric string is not identical to int", needle: "1", hay: []any{int64(1)}, strict: true},
		{name: "zero does not match a word", needle: int64(0), hay: []any{"a"}},
		{name: "string key is returned", needle: "b", hay: map[string]any{"y": "b"}, wantKey: "y", wantOK: true},
		{name: "int and float differ under strict", needle: int64(1), hay: []any{1.0}, strict: true},
		{name: "nested array compares by value", needle: []any{"a"}, hay: []any{[]any{"a"}}, strict: true, wantKey: int64(0), wantOK: true},
		{name: "null only matches null under strict", needle: nil, hay: []any{"", nil}, strict: true, wantKey: int64(1), wantOK: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key, ok := arrayFind(tc.needle, tc.hay, tc.strict)
			if ok != tc.wantOK {
				t.Fatalf("found = %v, want %v", ok, tc.wantOK)
			}
			if ok && !reflect.DeepEqual(key, tc.wantKey) {
				t.Errorf("key = %#v, want %#v", key, tc.wantKey)
			}
		})
	}
}

func BenchmarkArrayMerge(b *testing.B) {
	head := []any{"SELECT 1"}
	tail := []string{"a", "b", "c", "d"}

	b.Run("lists", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = phpArrayMerge(head, tail)
		}
	})
	b.Run("lists_legacy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = legacyArrayMerge(head, tail)
		}
	})

	assoc := model.NewArray()
	assoc.Set("a", 1)
	assoc.Set("b", 2)
	b.Run("string_keys", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = phpArrayMerge(assoc, tail)
		}
	})
}

// legacyArrayMerge is the pre-fast-path implementation, kept so the benchmark
// measures the change rather than asserting it (see docs/allocation-performance.md).
func legacyArrayMerge(arrs ...any) *model.Array {
	size := 0
	for _, a := range arrs {
		n, _ := model.LenValues(a)
		size += n
	}
	out := model.NewArraySize(size)
	for _, a := range arrs {
		model.RangeValues(a, func(k, v any) bool {
			if _, isInt := k.(int64); isInt {
				out.Append(v)
			} else {
				out.Set(k, v)
			}
			return true
		})
	}
	return out
}
