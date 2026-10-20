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
// is not the next index, which no caller and no behaviour can tell apart; see
// promote. Keys are opaque here, and normalising "1" to 1 is the caller's job
// (runner.normalizeKey).
type Array struct {
	list   []any // list mode only: element i is the value of key int64(i)
	keys   []any // map mode only: keys in insertion order
	values map[any]any
	nextID int64
}

// noIntKey is nextID's value for a map-mode array that has never held an
// integer key, which PHP distinguishes from one whose next index is 0.
//
// PHP 8.3 changed the rule: the first integer key inserted sets the next index
// to key+1 whatever its sign, and later keys only raise it, so [-5 => a] then
// $a[] lands on -4 where PHP 7 landed on 0. An array that never held one
// appends at 0. Those two states are both "nextID is not above any key", so
// they need telling apart, and math.MinInt64 is below every key a Set can see:
// the sentinel costs no field, and the `i >= a.nextID` comparison that raises
// the index reads it correctly without a special case.
//
// List mode does not use it. There nextID is len(list) by construction, and an
// array in list mode has held every integer key from 0 up, so promote carries
// the sentinel in only when it promotes an empty list.
const noIntKey = math.MinInt64

// NewArray returns an empty ordered array.
func NewArray() *Array {
	return &Array{nextID: noIntKey}
}

// NewArraySize returns an empty ordered array with room for n entries. Building
// an array of known size through it avoids the backing slice's growth
// reallocations (a 5-entry array grows 1->2->4->8), which is most of what an
// *Array costs while it stays in list mode.
func NewArraySize(n int) *Array {
	if n <= 0 {
		return NewArray()
	}
	return &Array{list: make([]any, 0, n), nextID: noIntKey}
}

// Reset empties the array while keeping its storage, so a per-request
// container (a superglobal, argv) refills into the buckets and slices the
// previous request grew, and reallocates none of them. A reset map-mode array
// stays in map mode with zero entries, which is observably the same as
// fresh; the cleared slices hold no stale references.
func (a *Array) Reset() {
	clear(a.list)
	a.list = a.list[:0]
	clear(a.keys)
	a.keys = a.keys[:0]
	clear(a.values)
	// The sentinel, not 0, is what makes the claim above true: a reset array
	// that held an integer key would otherwise append at 0 where a fresh one
	// appends from the first key the caller sets.
	a.nextID = noIntKey
}

// isList reports whether the array is still in list mode.
func (a *Array) isList() bool { return a.values == nil }

// promote switches the array from list mode to map mode, materialising the
// implicit integer keys. No other code creates the map or the key slice.
//
// The empty case reuses the list's backing array as the key slice, so a
// presized array whose first key is a string (json_decode of an object, the
// sorted introspection listings) still costs exactly the map. A non-empty list
// gets a fresh key slice by decision: a script may promote an array from inside
// its own foreach, and Range is walking the backing array we would otherwise
// be overwriting with keys.
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
		// In list mode nextID is always len(list), so an int64 key inside
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
		// Assigned and never incremented, because a list's next index is its
		// length by definition and the counter may still hold the sentinel.
		a.nextID = int64(len(a.list))
		return
	}
	if a.nextID == noIntKey {
		// Nothing has set the index, so this is the first integer key and PHP
		// answers 0.
		a.Set(int64(0), val)
		return
	}
	a.Set(a.nextID, val)
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
// express, and dropping the last one would still leave nextID past the end,
// which is also what PHP does, since unset never renumbers.
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
//
// It lives here and not in the shim because of the append index, which is
// the one piece of state a caller cannot reach. PHP decrements it only when the
// removed key is the one it would hand out next, so popping 9 from
// [5 => a, 9 => c] leaves the next append at 9, while popping 5 from
// [5 => a, 9 => c] leaves it at 10. Rebuilding the array from its entries
// cannot express that, because it loses the counter.
func (a *Array) Pop() (any, any, bool) {
	if a.Len() == 0 {
		return nil, nil, false
	}
	keys := a.Keys()
	key := keys[len(keys)-1]
	value, _ := a.Get(key)
	a.Delete(key)
	// The guard excludes the sentinel and never decrements it: an array holding
	// an integer key has had the index set, so the two cannot both be true, and
	// nextID-1 on the sentinel would wrap.
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

// Clear removes all entries and resets list indexing, returning the array to
// list mode.
func (a *Array) Clear() {
	a.list = nil
	a.keys = nil
	a.values = nil
	a.nextID = 0
}

// ToArray returns v as an *Array, converting a native collection if needed and
// passing an existing *Array through untouched. Use it at the point where PHP
// semantics genuinely require an array (mutation, `$a[] = v`), not to normalise
// arguments, since RangeValues reads every shape without allocating.
func ToArray(v any) *Array {
	if arr, ok := v.(*Array); ok && arr != nil {
		return arr
	}
	// A Go slice is already a list: its elements go straight into a list-mode
	// Array (one allocation for the copy) and pays no Set per element.
	if items, ok := v.([]any); ok {
		out := NewArraySize(len(items))
		out.list = append(out.list, items...)
		// An empty slice held no integer key, so it keeps the sentinel
		// NewArraySize set, and claims no next index of 0.
		if len(items) > 0 {
			out.nextID = int64(len(items))
		}
		return out
	}
	n, _ := LenValues(v)
	out := NewArraySize(n)
	RangeValues(v, func(key, val any) bool {
		if i, ok := key.(int64); ok && i == out.nextID {
			out.Append(val)
		} else {
			out.Set(key, val)
		}
		return true
	})
	return out
}
