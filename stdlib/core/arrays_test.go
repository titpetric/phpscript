package core

import (
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
