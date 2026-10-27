package model

// This file is the mapping between a PHP value and the Go value that carries
// it. PHP's types reach Go as native ones and a value flows in and out of a
// binding with no marshalling layer, and a registered Go function is callable
// from a script:
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
// Array, Object, Class and Func each have a file of their own beside this one.

// ArrayItemValue is a runtime (already-evaluated) array entry, the value-level
// counterpart of the ArrayItem AST node. The transpiled __array/__pair helpers
// produce these. Key is nil for list-style appends.
type ArrayItemValue struct {
	Key any
	Val any
}
