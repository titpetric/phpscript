package core

import (
	"errors"
	"reflect"
	"sort"

	"github.com/titpetric/phpscript/internal/arrayi64"
	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// init contributes the key-preserving sorts to stdlib.Register.
func init() {
	runner.RegisterBinding(registerArraySort)
}

// registerArraySort adds the half of PHP's sort family that keeps the
// key-to-value association: ksort/krsort order by key, asort/arsort by value,
// uasort/uksort by a script comparator. The other half (sort, rsort, usort)
// lives in arrays.go and throws the keys away and they can share
// sortValues and these cannot.
func registerArraySort(rt *runner.Runtime) {
	// ksort sorts $array in place by key ascending with PHP's default
	// comparison, keeping each key attached to its value.
	rt.RegisterFunc("ksort", phpKsort)
	// krsort sorts $array in place by key descending with PHP's default
	// comparison, keeping each key attached to its value.
	rt.RegisterFunc("krsort", phpKrsort)
	// asort sorts $array in place by value ascending with PHP's default
	// comparison, keeping each value attached to its key.
	rt.RegisterFunc("asort", phpAsort)
	// arsort sorts $array in place by value descending with PHP's default
	// comparison, keeping each value attached to its key.
	rt.RegisterFunc("arsort", phpArsort)
	// uasort sorts $array in place by value using the $callback comparator,
	// keeping each value attached to its key.
	rt.RegisterFunc("uasort", func(array any, callback any) (bool, error) {
		fn, ok := rt.Callable(callback)
		if !ok {
			return false, errors.New("uasort(): argument #2 ($callback) must be a valid callback")
		}
		return sortEntries("uasort", array, func(x, y arrayEntry) bool {
			return callbackLess(fn, x.val, y.val)
		})
	})
	// uksort sorts $array in place by key using the $callback comparator,
	// keeping each key attached to its value.
	rt.RegisterFunc("uksort", func(array any, callback any) (bool, error) {
		fn, ok := rt.Callable(callback)
		if !ok {
			return false, errors.New("uksort(): argument #2 ($callback) must be a valid callback")
		}
		return sortEntries("uksort", array, func(x, y arrayEntry) bool {
			return callbackLess(fn, x.key, y.key)
		})
	})
}

// phpKsort orders the entries by key, ascending.
func phpKsort(array any) (bool, error) {
	return sortEntries("ksort", array, func(x, y arrayEntry) bool { return sortLess(x.key, y.key) })
}

// phpKrsort is phpKsort in descending order.
func phpKrsort(array any) (bool, error) {
	return sortEntries("krsort", array, func(x, y arrayEntry) bool { return sortLess(y.key, x.key) })
}

// phpAsort orders the entries by value, ascending.
func phpAsort(array any) (bool, error) {
	return sortEntries("asort", array, func(x, y arrayEntry) bool { return sortLess(x.val, y.val) })
}

// phpArsort is phpAsort in descending order.
func phpArsort(array any) (bool, error) {
	return sortEntries("arsort", array, func(x, y arrayEntry) bool { return sortLess(y.val, x.val) })
}

// callbackLess adapts a script comparator (func(...any) (any, error), as
// rt.Callable returns it) to the boolean sort.SliceStable declares, the same
// way phpUsort does. It reads a comparator that raises as "not less" and a
// failing callback leaves the order it saw and aborts nothing mid-sort.
func callbackLess(fn func(...any) (any, error), x, y any) bool {
	r, err := fn(x, y)
	if err != nil {
		return false
	}
	return phpval.Int(r) < 0
}

// sortEntries is the key-aware counterpart of sortValues. It snapshots the
// entries with arrayEntries, sorts the snapshot, then rebuilds the array with
// Clear followed by arrayReplay in restore mode: arrayReplay puts every key
// back with Set, never Append, because Append would hand out fresh integer
// keys and turn a key-preserving sort into sort(). Sorting the snapshot, and
// not the storage, also means the rewrite never iterates what it overwrites.
//
// Divergence from PHP, and from sort()/rsort()/usort() next door: these six
// require a *model.Array and error on anything else, including the native Go
// slice a binding such as explode() or array_keys() returns. sortValues can
// sort a Go slice in place through its backing array because it only permutes
// values; a re-keying sort has nowhere to put the keys, and a Go slice has no
// keys to preserve in the first place and sorting one here would either be a
// plain sort() under another name or a mutation of a copy the script cannot
// observe. arrayTarget is the established precedent (array_splice, array_shift)
// and this follows it. Cast with (array) or build the value with array() first.
func sortEntries(name string, array any, less func(x, y arrayEntry) bool) (bool, error) {
	a, err := arrayTarget(name, array)
	if err != nil {
		return false, err
	}
	entries := arrayEntries(a)
	sort.SliceStable(entries, func(i, j int) bool { return less(entries[i], entries[j]) })
	a.Clear()
	arrayReplay(a, entries, false)
	return true, nil
}

// phpUsort sorts values in place using cmp, reindexing with integer keys (PHP
// semantics). cmp is the env-adapted comparator: func(...any)(any,error).
func phpUsort(cmp func(...any) (any, error), a any) bool {
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
// a 0-indexed list; it sorts a native Go slice through the backing array the
// script shares and both change in place as PHP expects.
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
	// rv indexes the same backing array sort.SliceStable swaps and reads stay
	// consistent with the ongoing sort.
	sort.SliceStable(a, func(i, j int) bool {
		return less(rv.Index(i).Interface(), rv.Index(j).Interface())
	})
	return true
}

// edgeValue is reset and end: the first or last value of the array, false when
// it holds none. PHP defines both through the internal pointer, which the array
// model here does not carry; every practical call reads them for the value, and
// this answers with that.
func edgeValue(array any, first bool) any {
	var out any = false
	found := false
	model.RangeValues(array, func(_, v any) bool {
		out = v
		found = true
		return !first
	})
	if !found {
		return false
	}
	return out
}
