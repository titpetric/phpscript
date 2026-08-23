package core

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/titpetric/phpscript/internal/arrayi64"
	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// init contributes the array shims to stdlib.Register.
func init() {
	runner.RegisterBinding(registerArrays)
}

// through model.RangeValues, so a script can pass either a PHP array or the
// native Go slice/map a binding returned. Those that build a fresh list return
// a []any (one allocation, presized) instead of an *model.Array (a struct, a
// map[any]any, a growing key slice and an interface box per key). The ones that
// preserve or merge keys still return *model.Array, because only *model.Array
// carries PHP's ordered hybrid-key semantics.
func registerArrays(rt *runner.Runtime) {
	rt.RegisterFunc("compact", phpCompact)
	// count returns the number of elements in $array; non-array values count as 0.
	rt.RegisterFunc("count", func(array any) int64 {
		n, _ := model.LenValues(array)
		return int64(n)
	})
	// in_array reports whether $needle occurs in $haystack, comparing values as strings; the $strict argument is accepted and ignored.
	rt.RegisterFunc("in_array", func(needle, haystack any, strict ...any) bool {
		found := false
		model.RangeValues(haystack, func(_, v any) bool {
			if phpval.String(v) == phpval.String(needle) {
				found = true
				return false
			}
			return true
		})
		return found
	})
	// array_unique returns $array with duplicate values removed, comparing values as strings and keeping the first occurrence and its key; the $flags argument is accepted and ignored.
	rt.RegisterFunc("array_unique", func(array any, flags ...any) *model.Array {
		n, _ := model.LenValues(array)
		out := model.NewArraySize(n)
		seen := make(map[string]bool, n)
		model.RangeValues(array, func(k, v any) bool {
			s := phpval.String(v)
			if !seen[s] {
				seen[s] = true
				out.Set(k, v)
			}
			return true
		})
		return out
	})
	// array_merge merges the given arrays into one; integer keys are renumbered and later string keys overwrite earlier ones.
	rt.RegisterFunc("array_merge", phpArrayMerge)
	// array_keys returns the keys of $array as a list; there is no $filter_value parameter.
	rt.RegisterFunc("array_keys", func(array any) []any {
		n, _ := model.LenValues(array)
		out := make([]any, 0, n)
		model.RangeValues(array, func(k, _ any) bool { out = append(out, k); return true })
		return out
	})
	// array_values returns the values of $array as a list indexed from zero.
	rt.RegisterFunc("array_values", func(array any) []any {
		n, _ := model.LenValues(array)
		out := make([]any, 0, n)
		model.RangeValues(array, func(_, v any) bool { out = append(out, v); return true })
		return out
	})
	// array_slice returns up to $length elements of $array starting at $offset, a negative $offset counting from the end; keys are discarded and reindexed from zero, and a negative $length yields an empty array.
	rt.RegisterFunc("array_slice", phpArraySlice)
	// array_splice removes $length elements of $array at $offset, inserts $replacement in their place, and returns the removed elements; a value that is not a script array is an error.
	rt.RegisterFunc("array_splice", phpArraySplice)
	// array_map returns a list of $callback applied to each value of $array; a single array is accepted and keys are not preserved.
	rt.RegisterFunc("array_map", func(callback any, array any) ([]any, error) {
		fn, ok := rt.Callable(callback)
		if !ok {
			return nil, errors.New("array_map(): argument #1 ($callback) must be a valid callback")
		}
		return phpArrayMap(fn, array)
	})
	// usort sorts $array in place using the $callback comparator, reindexing the keys from zero.
	rt.RegisterFunc("usort", func(array any, callback any) (bool, error) {
		fn, ok := rt.Callable(callback)
		if !ok {
			return false, errors.New("usort(): argument #2 ($callback) must be a valid callback")
		}
		return phpUsort(array, fn), nil
	})
	// sort sorts $array in place ascending with PHP's default comparison, discarding the keys and reindexing from zero.
	rt.RegisterFunc("sort", phpSort)
	// rsort sorts $array in place descending with PHP's default comparison, discarding the keys and reindexing from zero.
	rt.RegisterFunc("rsort", phpRsort)
}

// phpArrayMerge implements array_merge. PHP renumbers integer keys and lets
// later string keys win.
//
// This returns an *model.Array even when every input is a list. Returning a
// presized []any for that case is tempting, because it is the shape
// `call_user_func_array($fn, array_merge(...))` forwards with no copy, but
// rule 4 of docs/allocation-performance.md reserves *model.Array for values
// the script appends to, and `$x = array_merge($a, $b); $x[] = "z"` is
// ordinary PHP that a Go slice cannot serve (a slice cannot grow through the
// interface value holding it). Since model.Array gained its list mode, a
// merged list costs one struct allocation more than the slice and no map at
// all, so the appendability is close to free. Measured: 8 allocs either way.
func phpArrayMerge(arrs ...any) any {
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

// phpCompact builds an associative map from variables in the calling scope.
// Names that do not exist are omitted, matching PHP's compact().
func phpCompact(ctx context.Context, names ...string) map[string]any {
	out := make(map[string]any, len(names))
	scope, ok := runner.ScopeFromContext(ctx)
	if !ok {
		return out
	}
	for _, name := range names {
		if value, ok := scope.Get(name); ok {
			out[name] = value
		}
	}
	return out
}

// phpArraySplice removes (and optionally replaces) a run of elements, returning
// the removed ones as a []any. It resizes its argument, which a Go slice cannot
// do through the interface holding it, so this is one of the few shims that
// genuinely requires a *model.Array. Passing anything else is an error rather
// than a silent no-op.
func phpArraySplice(array any, offset int64, optional ...any) ([]any, error) {
	a, ok := array.(*model.Array)
	if !ok || a == nil {
		return nil, fmt.Errorf("array_splice: expected an array, got %T", array)
	}

	vals := phpval.Values(a)
	n := len(vals)

	start := int(offset)
	if start < 0 {
		start += n
	}
	if start < 0 {
		start = 0
	}
	if start > n {
		start = n
	}

	end := n
	var replacement []any
	switch len(optional) {
	case 0:
		// length omitted: remove through end of array
	case 1:
		end = phpArraySpliceEnd(start, n, optional[0])
	case 2:
		end = phpArraySpliceEnd(start, n, optional[0])
		replacement = phpArraySpliceReplacement(optional[1])
	default:
		end = phpArraySpliceEnd(start, n, optional[0])
		replacement = phpArraySpliceReplacement(optional[1])
	}

	removed := make([]any, end-start)
	copy(removed, vals[start:end])

	out := make([]any, 0, start+len(replacement)+(n-end))
	out = append(out, vals[:start]...)
	out = append(out, replacement...)
	out = append(out, vals[end:]...)

	a.Clear()
	for _, v := range out {
		a.Append(v)
	}

	return removed, nil
}

func phpArraySpliceEnd(start, n int, lengthArg any) int {
	if lengthArg == nil {
		return n
	}
	l := int(phpval.Int(lengthArg))
	if l >= 0 {
		end := start + l
		if end > n {
			end = n
		}
		return end
	}
	end := n + l
	if end < start {
		end = start
	}
	return end
}

func phpArraySpliceReplacement(rep any) []any {
	if rep == nil {
		return nil
	}
	if model.IsCollection(rep) {
		return phpval.Values(rep)
	}
	return []any{rep}
}

// phpArraySlice returns the selected run as a []any. PHP's array_slice
// reindexes integer keys, which this shim has always done for every key, so a
// list is a faithful representation of the result.
func phpArraySlice(array any, offset int64, length ...int64) []any {
	vals := phpval.Values(array)
	start := int(offset)
	if start < 0 {
		start += len(vals)
	}
	if start < 0 {
		start = 0
	}
	if start > len(vals) {
		return nil
	}
	end := len(vals)
	if len(length) > 0 {
		end = start + int(length[0])
		if end < start {
			end = start
		}
		if end > len(vals) {
			end = len(vals)
		}
	}
	out := make([]any, end-start)
	copy(out, vals[start:end])
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

// phpUsort sorts values in place using cmp, reindexing with integer keys (PHP
// semantics). cmp is the env-adapted comparator: func(...any)(any,error).
func phpUsort(a any, cmp func(...any) (any, error)) bool {
	return sortValues(a, func(x, y any) bool {
		r, err := cmp(x, y)
		if err != nil {
			return false
		}
		return phpval.Int(r) < 0
	})
}

// phpSort orders a list with PHP's default comparison (see phpval.Compare). Like
// sort(), it discards the keys and reindexes the result from zero.
func phpSort(array any) bool {
	if sortInt64Array(array, false) {
		return true
	}
	return sortValues(array, sortLess)
}

// phpRsort is phpSort in descending order.
func phpRsort(array any) bool {
	if sortInt64Array(array, true) {
		return true
	}
	return sortValues(array, func(x, y any) bool { return sortLess(y, x) })
}

func sortInt64Array(a any, desc bool) bool {
	arr, ok := a.(*model.Array)
	if !ok || arr == nil {
		return false
	}
	vals, ok := arr.Int64List()
	if !ok {
		return false
	}
	arrayi64.Sort(vals, len(vals))
	if desc {
		for i, j := 0, len(vals)-1; i < j; i, j = i+1, j-1 {
			vals[i], vals[j] = vals[j], vals[i]
		}
	}
	arr.ReplaceInt64List(vals)
	return true
}

// sortLess is the boolean sort.SliceStable wants, derived from phpval.Compare so
// that ties stay ties.
func sortLess(x, y any) bool {
	return phpval.Compare(x, y) < 0
}

// sortValues reorders a list in place using less. A *model.Array is rebuilt as
// a 0-indexed list; a native Go slice is sorted through its backing array,
// which the script shares, so both are mutated in place as PHP expects.
func sortValues(a any, less func(x, y any) bool) bool {
	switch target := a.(type) {
	case nil:
		return false
	case *model.Array:
		if target == nil {
			return false
		}
		vals := phpval.Values(target)
		sort.SliceStable(vals, func(i, j int) bool { return less(vals[i], vals[j]) })
		target.Clear()
		for _, v := range vals {
			target.Append(v)
		}
		return true
	case []any:
		sort.SliceStable(target, func(i, j int) bool { return less(target[i], target[j]) })
		return true
	}

	rv := reflect.ValueOf(a)
	if rv.Kind() != reflect.Slice {
		return false
	}
	// rv indexes the same backing array sort.SliceStable swaps, so reads stay
	// consistent with the ongoing sort.
	sort.SliceStable(a, func(i, j int) bool {
		return less(rv.Index(i).Interface(), rv.Index(j).Interface())
	})
	return true
}
