package modelutil

import (
	"reflect"

	"github.com/titpetric/phpscript/model"
)

// RangeValues iterates a collection in order, calling fn for each key/value
// pair until fn returns false. It accepts:
//
//	*model.Array    insertion order, hybrid int64/string keys (a list-mode
//	                array walks its []any directly, with no map lookup)
//	slice, array    int64 keys in index order
//	map             key order is Go's (unordered), keys as declared
//
// Anything else, nil included, iterates zero times. PHP's foreach over a
// non-array warns and continues, and fails nothing.
func RangeValues(v any, fn func(key, val any) bool) {
	switch x := v.(type) {
	case nil:
		return
	case []byte:
		// A byte slice is a PHP string, not a list of integers and it iterates
		// zero times the way a string does. See phpval.Bytes.
		return
	case *model.Array:
		if x != nil {
			x.Range(fn)
		}
		return
	case model.Collection:
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

// LenValues reports a collection's entry count and whether v is a collection
// at all. It backs count(): a non-collection reports (0, false) so callers can
// apply PHP's "count of a scalar" behaviour themselves.
func LenValues(v any) (int, bool) {
	switch x := v.(type) {
	case nil:
		return 0, false
	case []byte:
		// Not a collection and count() applies its scalar rule and not
		// answering a byte count. See phpval.Bytes.
		return 0, false
	case *model.Array:
		if x == nil {
			return 0, false
		}
		return x.Len(), true
	case model.Collection:
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
// *model.Array or a native Go slice or map. Strings and structs are not, matching
// is_array(). A []byte reads as a string, not a list; see phpval.Bytes.
func IsCollection(v any) bool {
	switch x := v.(type) {
	case *model.Array, []any, []string, map[string]any, []map[string]any:
		return true
	case model.Collection:
		return x != nil
	case nil, []byte:
		return false
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		return true
	}
	return false
}
