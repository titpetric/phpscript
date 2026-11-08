package modelutil

import (
	"fmt"
	"iter"
)

// Seq returns a collection as a range-over-func sequence. A caller then writes
// a `for key, val := range` loop in place of a closure.
func Seq(v any) iter.Seq2[any, any] {
	return func(yield func(key, val any) bool) {
		RangeValues(v, yield)
	}
}

// Keys returns a collection's keys in iteration order, and an empty slice for
// a value that is no collection.
func Keys(v any) []any {
	n, _ := LenValues(v)
	out := make([]any, 0, n)
	RangeValues(v, func(key, _ any) bool {
		out = append(out, key)
		return true
	})
	return out
}

// Values returns a collection's values in iteration order, and an empty slice
// for a value that is no collection.
func Values(v any) []any {
	n, _ := LenValues(v)
	out := make([]any, 0, n)
	RangeValues(v, func(_, val any) bool {
		out = append(out, val)
		return true
	})
	return out
}

// Typed returns a collection's values as a []T.
// It stops at the first entry holding another type and names the key.
func Typed[T any](v any) ([]T, error) {
	n, _ := LenValues(v)
	out := make([]T, 0, n)
	var failed error
	RangeValues(v, func(key, val any) bool {
		typed, ok := val.(T)
		if !ok {
			failed = fmt.Errorf("entry %v holds %T, want %T", key, val, out)
			return false
		}
		out = append(out, typed)
		return true
	})
	if failed != nil {
		return nil, failed
	}
	return out, nil
}

// Map returns a collection keyed by the string spelling of its keys, the shape
// a template and a JSON encoder read. A later entry wins on a collision.
func Map(v any) map[string]any {
	n, _ := LenValues(v)
	out := make(map[string]any, n)
	RangeValues(v, func(key, val any) bool {
		out[fmt.Sprint(key)] = val
		return true
	})
	return out
}
