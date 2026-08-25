package model

import (
	"fmt"
	"strconv"
)

// PHP runtime values are represented with plain Go types so that they flow
// transparently in and out of the expr-lang VM (which works against native Go
// values and reflection):
//
//	PHP null    -> nil
//	PHP bool    -> bool
//	PHP int     -> int64
//	PHP float   -> float64
//	PHP string  -> string
//	PHP array   -> *Array (ordered map, the PHP array is both list and dict)
//	PHP object  -> *Object
//	callable    -> Func / Go func value
//
// Keeping values as native Go types is what makes "forwarded symbols" work:
// any Go function or struct method registered on the runtime can be invoked
// from transpiled code with no marshalling layer.

// Array is PHP's ordered hash map. It preserves insertion order and allows both
// integer and string keys, so it doubles as list and dictionary.
//
// It has two internal representations and switches between them by itself:
//
//	list mode  values live in `list`, the key of element i is int64(i).
//	           `keys` and `values` are nil, so the array costs one slice.
//	map mode   `values` holds key->value and `keys` holds insertion order.
//
// A new array starts in list mode, which is what `$a[] = v` (Append) and a PHP
// list literal produce, and stays there for as long as every key so far is the
// dense sequence 0,1,...,n-1. The first key that breaks the invariant (a
// string key, a negative or sparse integer, an int that is not an int64)
// promotes the array to map mode, permanently. See promote.
//
// Nothing about the observable behaviour differs between the two modes;
// list mode exists only so that the common case does not allocate a
// map[any]any, a key slice, and an interface box per key. Keys are still
// treated as opaque: an Array never normalises "1" to 1 (its callers do, see
// runner.normalizeKey), and only an int64 key advances the append index.
type Array struct {
	list   []any // list mode only: element i is the value of key int64(i)
	keys   []any // map mode only: keys in insertion order
	values map[any]any
	nextID int64
}

// NewArray returns an empty ordered array.
func NewArray() *Array {
	return &Array{}
}

// NewArraySize returns an empty ordered array with room for n entries. Building
// an array of known size through it avoids the backing slice's growth
// reallocations (a 5-entry array grows 1->2->4->8), which is most of what an
// *Array costs while it stays in list mode.
func NewArraySize(n int) *Array {
	if n <= 0 {
		return NewArray()
	}
	return &Array{list: make([]any, 0, n)}
}

// isList reports whether the array is still in list mode.
func (a *Array) isList() bool { return a.values == nil }

// promote switches the array from list mode to map mode, materialising the
// implicit integer keys. It is the only place the map and the key slice are
// allocated.
//
// The empty case reuses the list's backing array as the key slice, so a
// presized array whose first key is a string (json_decode of an object, the
// sorted introspection listings) still costs exactly the map. A non-empty list
// gets a fresh key slice on purpose: a script may promote an array from inside
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
		a.nextID++
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
// unset). A list-mode array is promoted first: dropping an element from the
// middle would leave the remaining keys sparse, which list mode cannot express,
// and dropping the last one would still leave nextID past the end, which is
// also what PHP does, since unset never renumbers.
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
// It lives here rather than in the shim because of the append index, which is
// the one piece of state a caller cannot reach. PHP decrements it only when the
// removed key was the one it was about to hand out, so popping 9 from
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
	if i, ok := key.(int64); ok && i == a.nextID-1 {
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
// values. PHP integer keys are represented by their decimal string form.
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

// Clear removes all entries and resets list indexing, returning the array to
// list mode.
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
}

func (a *Array) Clear() {
	a.list = nil
	a.keys = nil
	a.values = nil
	a.nextID = 0
}

// Class is the resolved, runnable form of a ClassDecl: field defaults plus a
// method table keyed by method name.
//
// Statics are the declarations of `static $name = expr` properties; their
// values live in the runtime (one bag per class, created on first access) so
// that every instance and every static call observes the same storage.
type Class struct {
	Name string
	// Implements is every interface name the declaration listed, plus the names
	// those interfaces extend, lower-cased. It records which contracts the
	// class was checked against, and is what `instanceof` answers an interface
	// name from. No member arrives through it; see docs/design.md.
	Implements []string
	Fields     []Field
	Statics    []Field // static property declarations (Name + default Expr)
	Consts     []Field // class constants (Name + value Expr)
	Methods    map[string]*FuncDecl
}

// Object is a class instance: a property bag plus a pointer back to its class.
// Because methods live on the Class (resolved by the runtime), an Object passed
// into expr-lang exposes its Props for `$obj->field` style access.
type Object struct {
	Class *Class
	Props map[string]any
	ID    string
}

// NewObject builds an instance with field defaults applied.
func NewObject(c *Class) *Object {
	return &Object{Class: c, Props: map[string]any{}}
}

// SetID records the PHP variable receiving this constructed object.
func (o *Object) SetID(id string) {
	o.ID = id
}

// ArrayItemValue is a runtime (already-evaluated) array entry, the value-level
// counterpart of the ArrayItem AST node. The transpiled __array/__pair helpers
// produce these. Key is nil for list-style appends.
type ArrayItemValue struct {
	Key any
	Val any
}

// Func is a callable value: either a user-defined PHP function (Decl set) or a
// host Go function (Go set). Registered host functions use Go.
type Func struct {
	Decl *FuncDecl
	Go   any // an arbitrary Go func, invoked via reflection by the runtime
}
