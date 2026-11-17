package core

import (
	"reflect"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// phpArrayColumn projects one field from every row, returning []any or a keyed *model.Array.
func phpArrayColumn(array any, columnKey any, indexKey ...any) any {
	n, _ := modelutil.LenValues(array)

	var index any
	if len(indexKey) > 0 {
		index = indexKey[0]
	}
	if index == nil {
		out := make([]any, 0, n)
		modelutil.RangeValues(array, func(_, row any) bool {
			if v, ok := arrayColumnValue(row, columnKey); ok {
				out = append(out, v)
			}
			return true
		})
		return out
	}

	out := model.NewArraySize(n)
	modelutil.RangeValues(array, func(_, row any) bool {
		v, ok := arrayColumnValue(row, columnKey)
		if !ok {
			return true
		}
		// PHP appends a row whose index column is missing, and drops none of
		// them.
		if k, ok := arrayColumnValue(row, index); ok {
			out.Set(phpval.Key(k), v)
		} else {
			out.Append(v)
		}
		return true
	})
	return out
}

// arrayColumnValue reads one field of a row, a null key returning the whole row.
func arrayColumnValue(row any, key any) (any, bool) {
	if key == nil {
		return row, true
	}
	return arrayHasKey(row, key)
}

// phpInArray reports whether needle occurs in haystack.
func phpInArray(needle, haystack any, strict ...any) bool {
	_, found := arrayFind(needle, haystack, arrayStrict(strict))
	return found
}

// phpArraySearch returns the first matching key or false, a string key staying a string.
func phpArraySearch(needle, haystack any, strict ...any) any {
	key, found := arrayFind(needle, haystack, arrayStrict(strict))
	if !found {
		return false
	}
	return key
}

// arrayStrict reads the optional $strict argument the two searches share.
func arrayStrict(strict []any) bool {
	return len(strict) > 0 && phpval.Truthy(strict[0])
}

// arrayFind backs both searches, loose matching through phpval.Compare as PHP 8 compares.
func arrayFind(needle, haystack any, strict bool) (any, bool) {
	var key any
	found := false
	modelutil.RangeValues(haystack, func(k, v any) bool {
		if strict {
			if !arrayIdentical(needle, v) {
				return true
			}
		} else if phpval.Compare(needle, v) != 0 {
			return true
		}
		key, found = k, true
		return false
	})
	return key, found
}

// arrayIdentical is PHP's === at value level, falling back to reflect.DeepEqual for slices.
func arrayIdentical(x, y any) bool {
	if x == nil || y == nil {
		return x == nil && y == nil
	}
	tx := reflect.TypeOf(x)
	if tx != reflect.TypeOf(y) {
		return false
	}
	if tx.Comparable() {
		return x == y
	}
	return reflect.DeepEqual(x, y)
}

// phpArraySlice returns the selected run, reindexing integer keys unless $preserve_keys asks otherwise.
func phpArraySlice(array any, offset int64, args ...any) any {
	n, _ := modelutil.LenValues(array)
	keys := make([]any, 0, n)
	vals := make([]any, 0, n)
	modelutil.RangeValues(array, func(k, v any) bool {
		keys = append(keys, k)
		vals = append(vals, v)
		return true
	})

	start := int(offset)
	if start < 0 {
		start += len(vals)
		if start < 0 {
			start = 0
		}
	}
	if start > len(vals) {
		start = len(vals)
	}

	end := len(vals)
	if len(args) > 0 && args[0] != nil {
		if l := phpval.Int(args[0]); l < 0 {
			end = len(vals) + int(l)
		} else {
			end = start + int(l)
		}
		if end < start {
			end = start
		}
		if end > len(vals) {
			end = len(vals)
		}
	}

	preserve := len(args) > 1 && phpval.Truthy(args[1])
	keyed := preserve
	if !keyed {
		for _, k := range keys[start:end] {
			if _, ok := k.(string); ok {
				keyed = true
				break
			}
		}
	}

	if !keyed {
		out := make([]any, end-start)
		copy(out, vals[start:end])
		return out
	}

	out := model.NewArraySize(end - start)
	for i := start; i < end; i++ {
		if _, ok := keys[i].(string); ok || preserve {
			out.Set(keys[i], vals[i])
			continue
		}
		out.Append(vals[i])
	}
	return out
}

func phpArrayMap(fn func(...any) (any, error), a any) ([]any, error) {
	n, _ := modelutil.LenValues(a)
	out := make([]any, 0, n)
	var mapErr error
	modelutil.RangeValues(a, func(_, v any) bool {
		mapped, err := fn(v)
		if err != nil {
			mapErr = err
			return false
		}
		out = append(out, mapped)
		return true
	})
	if mapErr != nil {
		return nil, mapErr
	}
	return out, nil
}
