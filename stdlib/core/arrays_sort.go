package core

import (
	"errors"
	"sort"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/runner"
)

// init contributes the key-preserving sorts to stdlib.Register.
func init() {
	runner.RegisterBinding(registerArraySort)
}

// registerArraySort adds the half of PHP's sort family that keeps the
// key-to-value association: ksort/krsort order by key, asort/arsort by value,
// uasort/uksort by a script comparator. The other half (sort, rsort, usort)
// lives in arrays.go and throws the keys away, which is why they can share
// sortValues and these cannot.
func registerArraySort(rt *runner.Runtime) {
	// ksort sorts $array in place by key ascending with PHP's default comparison, keeping each key attached to its value.
	rt.RegisterFunc("ksort", phpKsort)
	// krsort sorts $array in place by key descending with PHP's default comparison, keeping each key attached to its value.
	rt.RegisterFunc("krsort", phpKrsort)
	// asort sorts $array in place by value ascending with PHP's default comparison, keeping each value attached to its key.
	rt.RegisterFunc("asort", phpAsort)
	// arsort sorts $array in place by value descending with PHP's default comparison, keeping each value attached to its key.
	rt.RegisterFunc("arsort", phpArsort)
	// uasort sorts $array in place by value using the $callback comparator, keeping each value attached to its key.
	rt.RegisterFunc("uasort", func(array any, callback any) (bool, error) {
		fn, ok := rt.Callable(callback)
		if !ok {
			return false, errors.New("uasort(): argument #2 ($callback) must be a valid callback")
		}
		return sortEntries("uasort", array, func(x, y arrayEntry) bool {
			return callbackLess(fn, x.val, y.val)
		})
	})
	// uksort sorts $array in place by key using the $callback comparator, keeping each key attached to its value.
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
// rt.Callable hands it over) to the boolean sort.SliceStable wants, the same
// way phpUsort does. A comparator that raises is treated as "not less", so a
// failing callback leaves the order it saw rather than aborting mid-sort.
func callbackLess(fn func(...any) (any, error), x, y any) bool {
	r, err := fn(x, y)
	if err != nil {
		return false
	}
	return phpval.Int(r) < 0
}

// sortEntries is the key-aware counterpart of sortValues. It snapshots the
// entries with arrayEntries, sorts the snapshot, then rebuilds the array with
// Clear followed by arrayReplay in restore mode: every key is written back with
// Set, never Append, because Append would hand out fresh integer keys and turn
// a key-preserving sort into sort(). Sorting the snapshot rather than the
// storage also means the rewrite never iterates what it is overwriting.
//
// Divergence from PHP, and from sort()/rsort()/usort() next door: these six
// require a *model.Array and error on anything else, including the native Go
// slice a binding such as explode() or array_keys() returns. sortValues can
// sort a Go slice in place through its backing array because it only permutes
// values; a re-keying sort has nowhere to put the keys, and a Go slice has no
// keys to preserve in the first place, so sorting one here would either be a
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
