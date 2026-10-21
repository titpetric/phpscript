package core

import (
	"fmt"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
)

// phpArrayMerge implements array_merge. PHP renumbers integer keys and lets
// later string keys win.
//
// This returns an *model.Array even when every input is a list. Returning a
// presized []any for that case is tempting, because
// `call_user_func_array($fn, array_merge(...))` forwards that shape with no
// copy, but rule 4 of docs/allocation-performance.md reserves *model.Array for
// values the script appends to, and `$x = array_merge($a, $b); $x[] = "z"` is
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
// shape that can: a *model.Array. This refuses a native slice from a binding
// (explode(), array_keys()) loudly, and never mutates one into a copy the
// script never sees. See "Known divergences from PHP" in docs/README.md.
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
// array_unshift: Append renumbers the integer keys from zero, string keys keep
// their name. Without it every key comes back unchanged, and array_pop needs
// that.
func arrayReplay(a *model.Array, entries []arrayEntry, renumber bool) {
	for _, e := range entries {
		if _, isInt := e.key.(int64); isInt && renumber {
			a.Append(e.val)
			continue
		}
		a.Set(e.key, e.val)
	}
}

// phpArrayShift removes the first element and renumbers the rest.
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
	// replay continues the numbering and never starts over.
	for _, v := range values {
		a.Append(v)
	}
	arrayReplay(a, entries, true)
	return int64(a.Len()), nil
}

// phpArrayPop removes the last element, keeping the keys of the rest. Unlike
// shift and unshift it does not renumber: PHP leaves the surviving keys alone,
// so popping 9 from [5 => a, 9 => c] leaves [5 => a], keeping the key 5.
//
// The append index is Array.Pop's business, the one piece of state a shim
// cannot reach.
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
