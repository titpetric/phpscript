package core

import (
	"reflect"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
)

// phpArrayColumn projects one field out of every row. The real caller is
// Database::get_all, which returns a []map[string]any, so this reads the rows
// through model.RangeValues and takes any collection shape.
//
// Without $index_key the result is a plain list, a presized []any. With one the
// function keys the result by a value taken from the rows, which can be a
// hybrid of int and string keys, so it returns an *model.Array (rule 4).
func phpArrayColumn(array any, columnKey any, indexKey ...any) any {
	n, _ := model.LenValues(array)

	var index any
	if len(indexKey) > 0 {
		index = indexKey[0]
	}
	if index == nil {
		out := make([]any, 0, n)
		model.RangeValues(array, func(_, row any) bool {
			if v, ok := arrayColumnValue(row, columnKey); ok {
				out = append(out, v)
			}
			return true
		})
		return out
	}

	out := model.NewArraySize(n)
	model.RangeValues(array, func(_, row any) bool {
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

// arrayColumnValue reads one field of a row. A null column key is PHP's "the
// whole row", so array_column($rows, null, "id") is a re-keying
// of the input.
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

// phpArraySearch returns the key of the first match, or false. The return type
// is PHP's union and not an int: a string-keyed array searches to a string
// key, and key int64(0) is only told apart from false with ===.
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

// arrayFind backs both in_array and array_search so the pair cannot disagree
// about what a match is. Loose matching resolves through phpval.Compare, the
// runtime's canonical comparison, and the source of PHP 8's rule that a
// non-numeric string does not equal 0.
func arrayFind(needle, haystack any, strict bool) (any, bool) {
	var key any
	found := false
	model.RangeValues(haystack, func(k, v any) bool {
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

// arrayIdentical is PHP's === at value level: the types must match before it
// compares the values. Go's == would panic on a slice or a map, so values it
// cannot compare directly fall back to reflect.DeepEqual.
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

// phpArraySlice returns the selected run of $array.
//
// PHP reindexes the integer keys of the result and keeps the string ones, and
// $preserve_keys asks it to keep the integers too. The result is a list only
// when nothing in the selected run carries a string key and nothing named the
// original integers; that case returns []any, which is the cheap shape
// (docs/allocation-performance.md), and the rest returns a *model.Array
// because the keys are then part of the result.
//
// This reads $length as PHP reads it: omitted or null runs to the end of the
// array, and a negative value stops that many elements short of the end rather
// than yielding nothing.
func phpArraySlice(array any, offset int64, args ...any) any {
	n, _ := model.LenValues(array)
	keys := make([]any, 0, n)
	vals := make([]any, 0, n)
	model.RangeValues(array, func(k, v any) bool {
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
	n, _ := model.LenValues(a)
	out := make([]any, 0, n)
	var mapErr error
	model.RangeValues(a, func(_, v any) bool {
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
