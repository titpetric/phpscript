package core

import (
	"context"
	"errors"
	"math"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// init contributes the array shims to stdlib.Register.
func init() {
	runner.RegisterBinding(registerArrays)
}

// registerArrays installs the array shims. They take `any`, not *model.Array,
// and read their input through model.RangeValues, so a script can pass either
// a PHP array or the native Go slice/map a binding returned. Those that build
// a fresh list return a []any (one allocation, presized) in place of an
// *model.Array (a struct, a map[any]any, a growing key slice and an interface
// box per key). The ones that preserve or merge keys still return
// *model.Array, because only *model.Array carries PHP's ordered hybrid-key
// semantics.
func registerArrays(rt *runner.Runtime) {
	rt.RegisterFunc("compact", phpCompact)
	// count returns the number of elements in $array; non-array values count as 0.
	rt.RegisterFunc("count", func(array any) int64 {
		n, _ := model.LenValues(array)
		return int64(n)
	})
	// in_array reports whether $needle occurs in $haystack, comparing loosely
	// with PHP 8 rules unless $strict is true, which compares types as well
	// as values.
	rt.RegisterFunc("in_array", phpInArray)
	// array_search returns the key of the first $haystack element equal to
	// $needle, or false when there is none; comparison is loose unless
	// $strict is true.
	rt.RegisterFunc("array_search", phpArraySearch)
	// array_unique returns $array with duplicate values removed, comparing
	// values as strings and keeping the first occurrence and its key; it takes
	// $flags and ignores it.
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
	// array_merge merges the given arrays into one; it renumbers integer keys
	// and later string keys overwrite earlier ones.
	rt.RegisterFunc("array_merge", phpArrayMerge)
	// reset returns the first value of $array, or false for an empty array;
	// there is no internal pointer here, so this is the value without the
	// rewind.
	rt.RegisterFunc("reset", func(array any) any { return edgeValue(array, true) })
	// end returns the last value of $array, or false for an empty array; there
	// is no internal pointer here, so this is the value without the seek.
	rt.RegisterFunc("end", func(array any) any { return edgeValue(array, false) })
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
	// array_slice returns up to $length elements of $array starting at
	// $offset, a negative $offset counting from the end and a negative
	// $length stopping that many short of it; it reindexes integer keys from
	// zero unless $preserve_keys is true, and keeps string keys either way.
	rt.RegisterFunc("array_slice", phpArraySlice)
	// array_splice removes $length elements of $array at $offset, inserts
	// $replacement in their place, and returns the removed elements; a value
	// that is not a script array is an error.
	rt.RegisterFunc("array_splice", phpArraySplice)
	// array_shift removes the first element of $array and returns it,
	// renumbering the integer keys from zero and leaving string keys alone;
	// an empty array returns null and a value that is not a script array is
	// an error.
	rt.RegisterFunc("array_shift", phpArrayShift)
	// array_unshift prepends the given values to $array and returns the new
	// element count, renumbering the integer keys from zero and leaving
	// string keys alone; a value that is not a script array is an error.
	rt.RegisterFunc("array_unshift", phpArrayUnshift)
	// array_pop removes the last element of $array and returns it, leaving
	// the remaining keys alone; an empty array returns null and a
	// value that is not a script array is an error.
	rt.RegisterFunc("array_pop", phpArrayPop)
	// array_push appends the given values to $array at the next integer keys
	// and returns the new element count; a value that is not a script array
	// is an error.
	rt.RegisterFunc("array_push", phpArrayPush)
	// array_map returns a list of $callback applied to each value of $array;
	// it takes a single array and keys are not preserved.
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
		return phpUsort(fn, array), nil
	})
	// sort sorts $array in place ascending with PHP's default comparison,
	// discarding the keys and reindexing from zero.
	rt.RegisterFunc("sort", phpSort)
	// rsort sorts $array in place descending with PHP's default comparison,
	// discarding the keys and reindexing from zero.
	rt.RegisterFunc("rsort", phpRsort)

	rt.SetConst("ARRAY_FILTER_USE_KEY", arrayFilterUseKey)
	rt.SetConst("ARRAY_FILTER_USE_BOTH", arrayFilterUseBoth)

	// array_key_exists reports whether $key is present in $array, which is
	// true even when the value stored there is null.
	rt.RegisterFunc("array_key_exists", phpArrayKeyExists)
	// array_filter returns the elements of $array for which $callback is
	// truthy, preserving the keys; without a $callback it filters the values
	// on their own truthiness, and $mode selects what the callback receives
	// (ARRAY_FILTER_USE_KEY the key, ARRAY_FILTER_USE_BOTH the value and the
	// key).
	rt.RegisterFunc("array_filter", func(array any, options ...any) (*model.Array, error) {
		var fn func(...any) (any, error)
		if len(options) > 0 && options[0] != nil {
			f, ok := rt.Callable(options[0])
			if !ok {
				return nil, errors.New("array_filter(): argument #2 ($callback) must be a valid callback")
			}
			fn = f
		}
		var mode int64
		if len(options) > 1 {
			mode = phpval.Int(options[1])
		}
		return phpArrayFilter(array, fn, mode)
	})
	// array_reduce folds $array with $callback, which receives the carry and
	// the value, starting from $initial and returning null for an empty
	// array.
	rt.RegisterFunc("array_reduce", func(array any, callback any, initial ...any) (any, error) {
		fn, ok := rt.Callable(callback)
		if !ok {
			return nil, errors.New("array_reduce(): argument #2 ($callback) must be a valid callback")
		}
		var carry any
		if len(initial) > 0 {
			carry = initial[0]
		}
		return phpArrayReduce(fn, array, carry)
	})
	// array_column returns the $column_key value of every row of $array,
	// keyed by each row's $index_key when the call passes one; a null
	// $column_key selects the whole row and it skips rows missing the column.
	rt.RegisterFunc("array_column", phpArrayColumn)
	// array_flip returns $array with its keys and values exchanged; it skips a
	// value that is neither an integer nor a string, as in PHP, but without
	// the warning.
	rt.RegisterFunc("array_flip", phpArrayFlip)
	// array_reverse returns $array in reverse order, renumbering the integer
	// keys from zero unless $preserve_keys is true; it keeps string keys
	// either way.
	rt.RegisterFunc("array_reverse", phpArrayReverse)
	// array_sum returns the sum of the values of $array as an int when every
	// value is an integer and as a float once one of them is a float or the
	// total overflows.
	rt.RegisterFunc("array_sum", phpArraySum)
	// range returns the list of values from $start to $end inclusive,
	// stepping by $step; two single-character strings produce a character
	// range, and any float endpoint or fractional step produces floats.
	rt.RegisterFunc("range", phpRange)
}

// The $mode values of array_filter, which are PHP's own constant values: 1
// passes the value and the key, 2 passes the key alone, 0 (the default) passes
// the value alone.
const (
	arrayFilterUseBoth = int64(1)
	arrayFilterUseKey  = int64(2)
)

// arrayHasKey looks up a normalised key in any collection shape. A *model.Array
// and a map[string]any answer in constant time; it walks anything else, because
// a native Go map of some other key type still needs a key-by-key comparison.
func arrayHasKey(array any, key any) (any, bool) {
	want := phpval.Key(key)
	switch a := array.(type) {
	case nil:
		return nil, false
	case *model.Array:
		if a == nil {
			return nil, false
		}
		return a.Get(want)
	case map[string]any:
		v, ok := a[phpval.String(key)]
		return v, ok
	}
	var found any
	ok := false
	model.RangeValues(array, func(k, v any) bool {
		if !arrayIdentical(phpval.Key(k), want) {
			return true
		}
		found, ok = v, true
		return false
	})
	return found, ok
}

// phpArrayKeyExists is isset() without the null check: the key exists even when
// the value stored there is null, which is the only reason the two functions
// are not the same function.
func phpArrayKeyExists(key, array any) bool {
	_, ok := arrayHasKey(array, key)
	return ok
}

// phpArrayFilter keeps the elements fn accepts. It returns an *model.Array
// because the result keeps the keys of the input, holes included, which is a
// shape only *model.Array carries (rule 4 of docs/allocation-performance.md).
// A nil fn is PHP's omitted callback: it keeps the truthy values.
func phpArrayFilter(array any, fn func(...any) (any, error), mode int64) (*model.Array, error) {
	n, _ := model.LenValues(array)
	out := model.NewArraySize(n)
	var filterErr error
	model.RangeValues(array, func(k, v any) bool {
		keep := false
		if fn == nil {
			keep = phpval.Truthy(v)
		} else {
			var (
				r   any
				err error
			)
			switch mode {
			case arrayFilterUseKey:
				r, err = fn(k)
			case arrayFilterUseBoth:
				r, err = fn(v, k)
			default:
				r, err = fn(v)
			}
			if err != nil {
				filterErr = err
				return false
			}
			keep = phpval.Truthy(r)
		}
		if keep {
			out.Set(phpval.Key(k), v)
		}
		return true
	})
	if filterErr != nil {
		return nil, filterErr
	}
	return out, nil
}

// phpArrayReduce folds the values left to right. The keys are not passed to the
// callback, matching PHP.
func phpArrayReduce(fn func(...any) (any, error), array any, carry any) (any, error) {
	var reduceErr error
	model.RangeValues(array, func(_, v any) bool {
		next, err := fn(carry, v)
		if err != nil {
			reduceErr = err
			return false
		}
		carry = next
		return true
	})
	if reduceErr != nil {
		return nil, reduceErr
	}
	return carry, nil
}

// phpArrayFlip exchanges keys and values. The new keys go through phpval.Key so
// that the flip of the value "1" is the key 1, the same key `$a[1] = x` would
// have written.
func phpArrayFlip(array any) *model.Array {
	n, _ := model.LenValues(array)
	out := model.NewArraySize(n)
	model.RangeValues(array, func(k, v any) bool {
		switch key := phpval.Key(v).(type) {
		case int64:
			out.Set(key, k)
		case string:
			out.Set(key, k)
		}
		return true
	})
	return out
}

// phpArrayReverse reverses the order of the elements. PHP drops integer keys
// and renumbers them from zero unless $preserve_keys is true, but it keeps
// string keys either way, so an array with any string key has to come back as
// an *model.Array. A renumbered all-integer-keyed array is a plain list, which
// is a presized []any (rule 4).
func phpArrayReverse(array any, preserveKeys ...any) any {
	preserve := len(preserveKeys) > 0 && phpval.Truthy(preserveKeys[0])

	n, _ := model.LenValues(array)
	keys := make([]any, 0, n)
	vals := make([]any, 0, n)
	strKeys := false
	model.RangeValues(array, func(k, v any) bool {
		key := phpval.Key(k)
		if _, isInt := key.(int64); !isInt {
			strKeys = true
		}
		keys = append(keys, key)
		vals = append(vals, v)
		return true
	})

	if !preserve && !strKeys {
		out := make([]any, len(vals))
		for i, v := range vals {
			out[len(vals)-1-i] = v
		}
		return out
	}

	out := model.NewArraySize(len(vals))
	for i := len(vals) - 1; i >= 0; i-- {
		if _, isInt := keys[i].(int64); isInt && !preserve {
			out.Append(vals[i])
			continue
		}
		out.Set(keys[i], vals[i])
	}
	return out
}

// phpArraySum adds the values up, preserving PHP's return type: an int64 while
// every value read as an integer, a float64 from the first float onwards. The
// promotion also happens on overflow, because PHP's integer arithmetic becomes
// float arithmetic there and never wraps.
func phpArraySum(array any) any {
	var (
		sum     int64
		total   float64
		isFloat bool
	)
	model.RangeValues(array, func(_, v any) bool {
		n := phpval.Number(v)
		if !isFloat {
			if x, ok := n.(int64); ok {
				if (x > 0 && sum > math.MaxInt64-x) || (x < 0 && sum < math.MinInt64-x) {
					isFloat = true
					total = float64(sum) + float64(x)
					return true
				}
				sum += x
				return true
			}
			isFloat, total = true, float64(sum)
		}
		total += phpval.Float(n)
		return true
	})
	if isFloat {
		return total
	}
	return sum
}

// phpCompact builds an associative map from variables in the calling scope.
// It omits names that do not exist, matching PHP's compact().
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
