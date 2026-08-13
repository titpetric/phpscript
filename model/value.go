package model

import "fmt"

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
type Array struct {
	keys   []any
	values map[any]any
	nextID int64
}

// NewArray returns an empty ordered array.
func NewArray() *Array {
	return &Array{values: map[any]any{}}
}

// Set assigns key=val, appending the key if new.
func (a *Array) Set(key, val any) {
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
	a.Set(a.nextID, val)
}

// Get returns the value for key and whether it existed.
func (a *Array) Get(key any) (any, bool) {
	v, ok := a.values[key]
	return v, ok
}

// Len reports the number of entries.
func (a *Array) Len() int { return len(a.keys) }

// Keys returns keys in insertion order.
func (a *Array) Keys() []any { return a.keys }

// Range iterates entries in insertion order.
func (a *Array) Range(fn func(key, val any) bool) {
	for _, k := range a.keys {
		if !fn(k, a.values[k]) {
			return
		}
	}
}

// Map returns the array as a string-keyed map for Go APIs that accept named
// values. PHP integer keys are represented by their decimal string form.
func (a *Array) Map() map[string]any {
	result := make(map[string]any, len(a.keys))
	for _, key := range a.keys {
		result[fmt.Sprint(key)] = a.values[key]
	}
	return result
}

// Clear removes all entries and resets list indexing.
func (a *Array) Clear() {
	a.keys = nil
	a.values = map[any]any{}
	a.nextID = 0
}

// Class is the resolved, runnable form of a ClassDecl: field defaults plus a
// method table keyed by method name.
type Class struct {
	Name    string
	Fields  []Field
	Consts  []Field // class constants (Name + value Expr)
	Methods map[string]*FuncDecl
}

// Object is a class instance: a property bag plus a pointer back to its class.
// Because methods live on the Class (resolved by the runtime), an Object passed
// into expr-lang exposes its Props for `$obj->field` style access.
type Object struct {
	Class *Class
	Props map[string]any
}

// NewObject builds an instance with field defaults applied.
func NewObject(c *Class) *Object {
	return &Object{Class: c, Props: map[string]any{}}
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
