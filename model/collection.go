package model

import "reflect"

// Values flow through phpscript as native Go types (see value.go). A PHP array
// literal is an *Array, but a value handed back by a forwarded Go function is
// just as likely to be a []string, a []map[string]any or a map[string]any —
// the runtime invokes bindings by reflection and boxes whatever they return.
//
// These helpers give every consumer (the runner's foreach and index access, the
// stdlib's array functions) a single way to walk a collection whatever its
// concrete type. That is what lets a binding return the cheapest representation
// of its data rather than paying to build an *Array nobody asked for: an
// *Array costs a struct, a map[any]any and a key slice, plus an interface box
// per key and per value, where the equivalent []string costs one allocation.
//
// *Array remains the right choice for values PHP mutates: it is the only shape
// with insertion-ordered hybrid int/string keys and support for `$a[] = v`.

// RangeValues iterates a collection in order, calling fn for each key/value
// pair until fn returns false. It accepts:
//
//	*Array          insertion order, hybrid int64/string keys
//	slice, array    int64 keys in index order
//	map             key order is Go's (unordered), keys as declared
//
// Anything else, nil included, iterates zero times — PHP's foreach over a
// non-array warns and continues rather than failing.
func RangeValues(v any, fn func(key, val any) bool) {
	switch x := v.(type) {
	case nil:
		return
	case *Array:
		if x != nil {
			x.Range(fn)
		}
		return
	case []any:
		for i, item := range x {
			if !fn(int64(i), item) {
				return
			}
		}
		return
	case []string:
		for i, item := range x {
			if !fn(int64(i), item) {
				return
			}
		}
		return
	case map[string]any:
		for key, item := range x {
			if !fn(key, item) {
				return
			}
		}
		return
	case []map[string]any:
		for i, item := range x {
			if !fn(int64(i), item) {
				return
			}
		}
		return
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if !fn(int64(i), rv.Index(i).Interface()) {
				return
			}
		}
	case reflect.Map:
		iter := rv.MapRange()
		for iter.Next() {
			if !fn(iter.Key().Interface(), iter.Value().Interface()) {
				return
			}
		}
	}
}

// LenValues reports a collection's entry count and whether v was a collection
// at all. It backs count(): a non-collection reports (0, false) so callers can
// apply PHP's "count of a scalar" behaviour themselves.
func LenValues(v any) (int, bool) {
	switch x := v.(type) {
	case nil:
		return 0, false
	case *Array:
		if x == nil {
			return 0, false
		}
		return x.Len(), true
	case []any:
		return len(x), true
	case []string:
		return len(x), true
	case map[string]any:
		return len(x), true
	case []map[string]any:
		return len(x), true
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		return rv.Len(), true
	}
	return 0, false
}

// IsCollection reports whether v is array-like from PHP's point of view: an
// *Array or a native Go slice or map. Strings and structs are not, matching
// is_array().
func IsCollection(v any) bool {
	switch v.(type) {
	case *Array, []any, []string, map[string]any, []map[string]any:
		return true
	case nil:
		return false
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		return true
	}
	return false
}

// ToArray returns v as an *Array, converting a native collection if needed and
// passing an existing *Array through untouched. Use it at the point where PHP
// semantics genuinely require an array (mutation, `$a[] = v`), not to normalise
// arguments — RangeValues reads every shape without allocating.
func ToArray(v any) *Array {
	if arr, ok := v.(*Array); ok && arr != nil {
		return arr
	}
	n, _ := LenValues(v)
	out := NewArraySize(n)
	RangeValues(v, func(key, val any) bool {
		if i, ok := key.(int64); ok && i == out.nextID {
			out.Append(val)
		} else {
			out.Set(key, val)
		}
		return true
	})
	return out
}
