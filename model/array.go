package model

import (
	"fmt"
	"math"
	"strconv"
)

// Array is PHP's ordered hash map: insertion order, integer and string keys,
// list and dictionary in one value.
//
// It starts as a dense slice and promotes itself to a map on the first key that
// is not the next index, which callers cannot observe; see promote. Keys are opaque here, and normalising "1" to 1 is the caller's job
// (runner.normalizeKey).
type Array struct {
	list   []any // list mode only: element i is the value of key int64(i)
	keys   []any // map mode only: keys in insertion order
	values map[any]any
	nextID int64
}

// noIntKey is nextID's value for a map-mode array holding no integer key.
const noIntKey = math.MinInt64

// NewArray returns an empty ordered array.
func NewArray() *Array {
	return &Array{nextID: noIntKey}
}

// NewArraySize returns an empty ordered array with room for n entries. The
// presize drops the backing slice's growth reallocations in list mode.
func NewArraySize(n int) *Array {
	if n <= 0 {
		return NewArray()
	}
	return &Array{list: make([]any, 0, n), nextID: noIntKey}
}

// Reset empties the array, keeping the storage the next request refills into.
func (a *Array) Reset() {
	clear(a.list)
	a.list = a.list[:0]
	clear(a.keys)
	a.keys = a.keys[:0]
	clear(a.values)
	// The sentinel, not 0, makes the claim above true: a reset array that held
	// an integer key would otherwise append at 0. A fresh array appends from
	// the first key the caller sets.
	a.nextID = noIntKey
}

// isList reports whether the array is still in list mode.
func (a *Array) isList() bool { return a.values == nil }

// promote switches list mode to map mode, materialising the implicit integer keys.
func (a *Array) promote() {
	n := len(a.list)
	size := n
	if c := cap(a.list); c > size {
		size = c
	}
	a.values = make(map[any]any, size)
	for i, v := range a.list {
		a.values[int64(i)] = v
	}
	if n == 0 {
		a.keys = a.list[:0]
	} else {
		a.keys = make([]any, n, size)
		for i := range a.keys {
			a.keys[i] = int64(i)
		}
	}
	a.list = nil
}

// Set assigns key=val, appending the key if new.
func (a *Array) Set(key, val any) {
	if a.isList() {
		// In list mode nextID is always len(list) and an int64 key inside
		// [0, len] either overwrites an element or extends the list by one.
		if i, ok := key.(int64); ok && i >= 0 && i <= int64(len(a.list)) {
			if i == int64(len(a.list)) {
				a.list = append(a.list, val)
				a.nextID = i + 1
			} else {
				a.list[i] = val
			}
			return
		}
		a.promote()
	}
	if _, ok := a.values[key]; !ok {
		a.keys = append(a.keys, key)
	}
	a.values[key] = val
	if i, ok := key.(int64); ok && i >= a.nextID {
		a.nextID = i + 1
	}
}

// Append adds val at the next integer index (PHP `$a[] = v`).
func (a *Array) Append(val any) {
	if a.isList() {
		a.list = append(a.list, val)
		// Assigned without increment, since a list's next index equals its
		// length by definition. The counter may still hold the sentinel.
		a.nextID = int64(len(a.list))
		return
	}
	if a.nextID == noIntKey {
		// The index holds no value yet and this is the first integer key and PHP
		// answers 0.
		a.Set(int64(0), val)
		return
	}
	a.Set(a.nextID, val)
}

// NextIndex is the integer key the next Append lands on.
func (a *Array) NextIndex() int64 {
	return a.nextID
}

// AppendAll adds every item at the next integer index, in one copy where the
// array is still a list.
//
// A Go slice is already a list and the fast path pays one allocation for the
// whole copy where Append per element pays on each growth step.
func (a *Array) AppendAll(items []any) {
	if len(items) == 0 {
		return
	}
	if !a.isList() {
		for _, item := range items {
			a.Append(item)
		}
		return
	}
	a.list = append(a.list, items...)
	a.nextID = int64(len(a.list))
}

// Get returns the value for key and whether it existed.
func (a *Array) Get(key any) (any, bool) {
	if a.isList() {
		if i, ok := key.(int64); ok && i >= 0 && i < int64(len(a.list)) {
			return a.list[i], true
		}
		return nil, false
	}
	v, ok := a.values[key]
	return v, ok
}

// Delete removes key, preserving the order of the entries around it (PHP's
// unset). A list-mode array promotes itself first: dropping an element from
// the middle would leave the remaining keys sparse, which list mode cannot
// express, and dropping the last element would still leave nextID past the
// end, matching PHP, since unset never renumbers.
func (a *Array) Delete(key any) {
	if a.isList() {
		i, ok := key.(int64)
		if !ok || i < 0 || i >= int64(len(a.list)) {
			return
		}
		if i == int64(len(a.list))-1 {
			a.list = a.list[:i]
			return
		}
		a.promote()
	}
	if _, ok := a.values[key]; !ok {
		return
	}
	delete(a.values, key)
	for i, k := range a.keys {
		if k == key {
			a.keys = append(a.keys[:i], a.keys[i+1:]...)
			break
		}
	}
}

// Pop removes the last entry and returns its key and value, PHP's array_pop.
// The append index drops only when the removed key equals the next key it
// would hand out: popping 9 from [5 => a, 9 => c] leaves the next append at 9.
func (a *Array) Pop() (any, any, bool) {
	if a.Len() == 0 {
		return nil, nil, false
	}
	keys := a.Keys()
	key := keys[len(keys)-1]
	value, _ := a.Get(key)
	a.Delete(key)
	// An array holding an integer key has set the index. The two cannot both
	// hold, and nextID-1 on the sentinel would wrap. The guard excludes the
	// sentinel.
	if i, ok := key.(int64); ok && a.nextID != noIntKey && i == a.nextID-1 {
		a.nextID = i
	}
	return key, value, true
}

// Len reports the number of entries.
func (a *Array) Len() int {
	if a.isList() {
		return len(a.list)
	}
	return len(a.keys)
}

// Keys returns keys in insertion order. A list-mode array materialises them on
// each call, since it does not store them.
func (a *Array) Keys() []any {
	if a.isList() {
		keys := make([]any, len(a.list))
		for i := range keys {
			keys[i] = int64(i)
		}
		return keys
	}
	return a.keys
}

// Range iterates entries in insertion order.
func (a *Array) Range(fn func(key, val any) bool) {
	if a.isList() {
		for i, v := range a.list {
			if !fn(int64(i), v) {
				return
			}
		}
		return
	}
	for _, k := range a.keys {
		if !fn(k, a.values[k]) {
			return
		}
	}
}

// Map returns the array as a string-keyed map for Go APIs that accept named
// values. A PHP integer key becomes its decimal string form.
func (a *Array) Map() map[string]any {
	if a.isList() {
		result := make(map[string]any, len(a.list))
		for i, v := range a.list {
			result[strconv.Itoa(i)] = v
		}
		return result
	}
	result := make(map[string]any, len(a.keys))
	for _, key := range a.keys {
		result[fmt.Sprint(key)] = a.values[key]
	}
	return result
}

// Int64List reports whether a is a dense list of int64 values and returns a
// copy of those values.
func (a *Array) Int64List() ([]int64, bool) {
	if a == nil || !a.isList() {
		return nil, false
	}
	out := make([]int64, len(a.list))
	for i, v := range a.list {
		n, ok := v.(int64)
		if !ok {
			return nil, false
		}
		out[i] = n
	}
	return out, true
}

// ReplaceInt64List puts vals back as a dense int64 list.
func (a *Array) ReplaceInt64List(vals []int64) {
	a.keys = nil
	a.values = nil
	a.list = make([]any, len(vals))
	for i, v := range vals {
		a.list[i] = v
	}
	a.nextID = int64(len(vals))
	if len(vals) == 0 {
		a.nextID = noIntKey
	}
}

// Clear removes all entries and returns the array to list mode.
func (a *Array) Clear() {
	a.list = nil
	a.keys = nil
	a.values = nil
	a.nextID = 0
}
