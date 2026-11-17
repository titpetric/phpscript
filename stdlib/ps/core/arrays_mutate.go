package core

import (
	"fmt"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// phpArrayMerge implements array_merge, renumbering integer keys and letting later string keys win.
func phpArrayMerge(arrs ...any) any {
	size := 0
	for _, a := range arrs {
		n, _ := modelutil.LenValues(a)
		size += n
	}

	out := model.NewArraySize(size)
	for _, a := range arrs {
		modelutil.RangeValues(a, func(k, v any) bool {
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

// phpArraySplice removes a run of elements and returns them, refusing anything but a *model.Array.
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
	if modelutil.IsCollection(rep) {
		return phpval.Values(rep)
	}
	return []any{rep}
}

// arrayEntry is one key/value pair of a snapshot taken before the array is rewritten.
type arrayEntry struct {
	key any
	val any
}

// arrayTarget is the four mutators' shared guard, refusing a native slice for want of *model.Array.
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

// arrayReplay repopulates a cleared array, renumbering integer keys from zero when renumber is true.
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
	// Appending the new values first leaves them holding keys 0..n-1 and the
	// replay continues the numbering and never starts over.
	for _, v := range values {
		a.Append(v)
	}
	arrayReplay(a, entries, true)
	return int64(a.Len()), nil
}

// phpArrayPop removes the last element and keeps the surviving keys, unlike shift and unshift.
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

// phpArrayPush appends values at the next integer keys and returns the new count, re-keying nothing.
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
