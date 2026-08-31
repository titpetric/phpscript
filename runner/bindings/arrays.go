package bindings

import (
	"errors"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// init contributes the set-shaped array functions to stdlib.Register.
func init() {
	runner.RegisterBinding(registerArrays)
}

// registerArrays installs the array functions that build a new array from one
// or two given ones. They read through model.RangeValues, so a script can pass
// either a PHP array or the native Go slice or map a binding returned, which is
// what stdlib/core/arrays.go does for the functions it carries.
func registerArrays(rt *runner.Runtime) {
	// array_fill returns an array of $count copies of $value, keyed from $start_index upwards; php 8 raises a ValueError below zero and this clamps to the empty array, which is what a count of zero already answers.
	rt.RegisterFunc("array_fill", func(start_index, count int64, value any) *model.Array {
		if count < 0 {
			count = 0
		}
		out := model.NewArraySize(int(count))
		for i := int64(0); i < count; i++ {
			out.Set(start_index+i, value)
		}
		return out
	})

	// array_fill_keys returns an array whose keys are the values of $keys and whose every value is $value; a key repeated in $keys lands once, as the later assignment overwrites the earlier.
	rt.RegisterFunc("array_fill_keys", func(keys any, value any) *model.Array {
		n, _ := model.LenValues(keys)
		out := model.NewArraySize(n)
		model.RangeValues(keys, func(_, k any) bool {
			out.Set(phpval.Key(k), value)
			return true
		})
		return out
	})

	// array_combine returns an array keyed by the values of $keys and valued by the values of $values, paired in order; the two must hold the same number of entries.
	rt.RegisterFunc("array_combine", func(keys, values any) (*model.Array, error) {
		keyList := valueList(keys)
		valList := valueList(values)
		if len(keyList) != len(valList) {
			return nil, errors.New("array_combine(): Argument #1 ($keys) must have the same number of elements as argument #2 ($values)")
		}

		out := model.NewArraySize(len(keyList))
		for i, k := range keyList {
			out.Set(phpval.Key(k), valList[i])
		}
		return out, nil
	})

	// array_chunk splits $array into arrays of at most $length entries; the chunks are keyed from zero either way, and $preserve_keys decides whether the entries inside them keep their own keys or are renumbered.
	rt.RegisterFunc("array_chunk", func(array any, length int64, preserve_keys ...bool) (*model.Array, error) {
		if length < 1 {
			return nil, errors.New("array_chunk(): Argument #2 ($length) must be greater than 0")
		}
		keep := len(preserve_keys) > 0 && preserve_keys[0]

		out := model.NewArray()
		var chunk *model.Array
		model.RangeValues(array, func(k, v any) bool {
			if chunk == nil {
				chunk = model.NewArraySize(int(length))
			}
			if keep {
				chunk.Set(k, v)
			} else {
				chunk.Append(v)
			}
			if int64(chunk.Len()) == length {
				out.Append(chunk)
				chunk = nil
			}
			return true
		})
		// The last chunk is short unless the count divided evenly.
		if chunk != nil {
			out.Append(chunk)
		}
		return out, nil
	})

	// array_diff returns the entries of $array whose value is in none of the other arrays, keys kept; values are compared as strings, which is php's own rule and the reason 0 and "0" are the same entry here.
	rt.RegisterFunc("array_diff", func(array any, others ...any) *model.Array {
		return filterByValue(array, others, false)
	})

	// array_intersect returns the entries of $array whose value is in every one of the other arrays, keys kept; values are compared as strings, as array_diff compares them.
	rt.RegisterFunc("array_intersect", func(array any, others ...any) *model.Array {
		return filterByValue(array, others, true)
	})

	// array_diff_key returns the entries of $array whose key is in none of the other arrays, values untouched and never compared.
	rt.RegisterFunc("array_diff_key", func(array any, others ...any) *model.Array {
		return filterByKey(array, others, false)
	})

	// array_intersect_key returns the entries of $array whose key is in every one of the other arrays, values untouched and never compared.
	rt.RegisterFunc("array_intersect_key", func(array any, others ...any) *model.Array {
		return filterByKey(array, others, true)
	})

	// array_key_first returns the first key of $array without moving anything, or null when it is empty.
	rt.RegisterFunc("array_key_first", func(array any) any {
		return edgeKey(array, true)
	})

	// array_key_last returns the last key of $array, or null when it is empty.
	rt.RegisterFunc("array_key_last", func(array any) any {
		return edgeKey(array, false)
	})

	// array_is_list reports whether $array is keyed by the integers 0 to count-1 in that order, which is what makes it a list rather than a map; an empty array is a list.
	rt.RegisterFunc("array_is_list", func(array any) bool {
		want := int64(0)
		isList := true
		model.RangeValues(array, func(k, _ any) bool {
			i, ok := phpval.Key(k).(int64)
			if !ok || i != want {
				isList = false
				return false
			}
			want++
			return true
		})
		return isList
	})
}

// valueList reads the values of an array in order. array_combine needs both
// sides indexable at the same offset, which a range alone does not give.
func valueList(array any) []any {
	n, _ := model.LenValues(array)
	out := make([]any, 0, n)
	model.RangeValues(array, func(_, v any) bool {
		out = append(out, v)
		return true
	})
	return out
}

// filterByValue is array_diff and array_intersect: same walk, opposite verdict.
//
// The comparison is by string, which is PHP's documented rule for this family -
// (string)$a === (string)$b - and not the loose == the sorts use. It is why
// array_diff([0], ["0"]) is empty.
func filterByValue(array any, others []any, keep bool) *model.Array {
	seen := make(map[string]int, len(others))
	for _, other := range others {
		// Counting the arrays a value appears in, rather than marking it
		// present, is what lets intersect require all of them from the same
		// pass diff uses to require none.
		inThis := make(map[string]bool)
		model.RangeValues(other, func(_, v any) bool {
			inThis[phpval.String(v)] = true
			return true
		})
		for value := range inThis {
			seen[value]++
		}
	}

	out := model.NewArray()
	model.RangeValues(array, func(k, v any) bool {
		if wanted(seen[phpval.String(v)], len(others), keep) {
			out.Set(k, v)
		}
		return true
	})
	return out
}

// wanted turns "how many of the other arrays hold this" into the verdict.
//
// The two are not each other's negation: intersect wants a value present in
// every other array, diff wants it in NONE of them. With one other array those
// coincide, which is why the difference only shows once a third is passed -
// array_diff($a, $b, $c) drops a value $b holds even though $c does not.
func wanted(count, others int, keep bool) bool {
	if keep {
		return count == others
	}
	return count == 0
}

// filterByKey is array_diff_key and array_intersect_key. Keys are normalised
// through phpval.Key first, so the integer 1 and the string "1" are the one key
// PHP treats them as.
func filterByKey(array any, others []any, keep bool) *model.Array {
	seen := make(map[any]int, len(others))
	for _, other := range others {
		inThis := make(map[any]bool)
		model.RangeValues(other, func(k, _ any) bool {
			inThis[phpval.Key(k)] = true
			return true
		})
		for key := range inThis {
			seen[key]++
		}
	}

	out := model.NewArray()
	model.RangeValues(array, func(k, v any) bool {
		if wanted(seen[phpval.Key(k)], len(others), keep) {
			out.Set(k, v)
		}
		return true
	})
	return out
}

// edgeKey answers the first or last key. There is no internal pointer in this
// runtime, which is what array_key_first and array_key_last are for: they read
// an edge without one, where PHP's older spelling was reset() plus key().
func edgeKey(array any, first bool) any {
	var found any
	has := false
	model.RangeValues(array, func(k, _ any) bool {
		found = k
		has = true
		return !first
	})
	if !has {
		return nil
	}
	return found
}
