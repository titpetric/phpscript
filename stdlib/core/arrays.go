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
	// in_array reports whether $needle occurs in $haystack, comparing loosely with PHP 8 rules unless $strict is true, which compares types as well as values.
	rt.RegisterFunc("in_array", phpInArray)
	// array_search returns the key of the first $haystack element equal to $needle, or false when there is none; comparison is loose unless $strict is true.
	rt.RegisterFunc("array_search", phpArraySearch)
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
	// array_shift removes the first element of $array and returns it, renumbering the integer keys from zero and leaving string keys alone; an empty array returns null and a value that is not a script array is an error.
	rt.RegisterFunc("array_shift", phpArrayShift)
	// array_unshift prepends the given values to $array and returns the new element count, renumbering the integer keys from zero and leaving string keys alone; a value that is not a script array is an error.
	rt.RegisterFunc("array_unshift", phpArrayUnshift)
	// array_pop removes the last element of $array and returns it, leaving the remaining keys as they were; an empty array returns null and a value that is not a script array is an error.
	rt.RegisterFunc("array_pop", phpArrayPop)
	// array_push appends the given values to $array at the next integer keys and returns the new element count; a value that is not a script array is an error.
	rt.RegisterFunc("array_push", phpArrayPush)
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

// arrayEntry is one key/value pair of a snapshot taken before the array is
// rewritten, so the replay is never iterating the storage it overwrites.
type arrayEntry struct {
	key any
	val any
}

// arrayTarget is the shared guard of the four mutators. They resize their
// argument, and a Go slice cannot grow through the interface value holding it,
// so - like array_splice, whose precedent this follows - they require the one
// shape that can: a *model.Array. A native slice from a binding (explode(),
// array_keys()) is rejected loudly rather than mutated into a copy the script
// never sees. See "Known divergences from PHP" in docs/README.md.
func arrayTarget(name string, array any) (*model.Array, error) {
	a, ok := array.(*model.Array)
	if !ok || a == nil {
		return nil, fmt.Errorf("%s: expected an array, got %T", name, array)
	}
	return a, nil
}

// arrayEntries snapshots the array in insertion order.
func arrayEntries(a *model.Array) []arrayEntry {
	out := make([]arrayEntry, 0, a.Len())
	a.Range(func(k, v any) bool {
		out = append(out, arrayEntry{key: k, val: v})
		return true
	})
	return out
}

// arrayReplay appends entries to a, which the caller has cleared. With
// renumber set it applies PHP's re-keying rule for array_shift and
// array_unshift: integer keys are handed out again from zero through Append,
// string keys keep their name. Without it every key is restored as it was,
// which is what array_pop needs.
func arrayReplay(a *model.Array, entries []arrayEntry, renumber bool) {
	for _, e := range entries {
		if _, isInt := e.key.(int64); isInt && renumber {
			a.Append(e.val)
			continue
		}
		a.Set(e.key, e.val)
	}
}

// phpArrayShift removes the first element and renumbers what is left.
func phpArrayShift(array any) (any, error) {
	a, err := arrayTarget("array_shift", array)
	if err != nil {
		return nil, err
	}
	entries := arrayEntries(a)
	if len(entries) == 0 {
		return nil, nil
	}
	a.Clear()
	arrayReplay(a, entries[1:], true)
	return entries[0].val, nil
}

// phpArrayUnshift prepends values and renumbers, returning the new count.
func phpArrayUnshift(array any, values ...any) (int64, error) {
	a, err := arrayTarget("array_unshift", array)
	if err != nil {
		return 0, err
	}
	entries := arrayEntries(a)
	a.Clear()
	// Appending the new values first leaves them holding keys 0..n-1, so the
	// replay continues the numbering rather than starting over.
	for _, v := range values {
		a.Append(v)
	}
	arrayReplay(a, entries, true)
	return int64(a.Len()), nil
}

// phpArrayPop removes the last element, keeping the keys of the rest. Unlike
// shift and unshift it does not renumber: PHP leaves the surviving keys alone,
// so popping 9 from [5 => a, 9 => c] leaves [5 => a] rather than [0 => a].
//
// The append index is Array.Pop's business, since it is the one piece of state
// a shim cannot reach.
func phpArrayPop(array any) (any, error) {
	a, err := arrayTarget("array_pop", array)
	if err != nil {
		return nil, err
	}
	_, value, ok := a.Pop()
	if !ok {
		return nil, nil
	}
	return value, nil
}

// phpArrayPush appends values at the next integer keys, returning the new
// count. Unlike the other three it does not re-key: PHP leaves the existing
// keys and the append index alone.
func phpArrayPush(array any, values ...any) (int64, error) {
	a, err := arrayTarget("array_push", array)
	if err != nil {
		return 0, err
	}
	for _, v := range values {
		a.Append(v)
	}
	return int64(a.Len()), nil
}

// phpInArray reports whether needle occurs in haystack.
func phpInArray(needle, haystack any, strict ...any) bool {
	_, found := arrayFind(needle, haystack, arrayStrict(strict))
	return found
}

// phpArraySearch returns the key of the first match, or false. The return type
// is PHP's union rather than an int: a string-keyed array searches to a string
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
// about what a match is. Loose matching goes through phpval.Compare, the
// runtime's canonical comparison, which is where PHP 8's rule that a
// non-numeric string does not equal 0 comes from.
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

// arrayIdentical is PHP's === at value level: the types must match before the
// values are looked at. Go's == would panic on a slice or a map, so values it
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
