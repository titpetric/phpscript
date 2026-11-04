package model

import (
	"maps"
	"slices"
)

// This file is the go runtime's name tables: the namespace a script imports to
// bind go's builtins, the names that import binds, and the conversion a
// builtin type name applies. The parser reads them to bind a name and fold a
// literal while it parses, the bindings read them to register and to convert,
// and this file spells the list once.

// GoNamespace is the namespace a script imports to bind go's builtins.
const GoNamespace = `Runtime\Go`

// goBuiltins are the names the import binds where a script writes them
// unqualified. An import is the more specific name: a file that imported the
// go runtime wrote `min(...)` for go's, and `\min(...)` is php's, the way
// `\strlen` is php's whatever the file imported. Three of these shadow a php
// function - copy, min and max - and php's print is no function phpscript
// registers at all.
var goBuiltins = map[string]struct{}{
	"append":  {},
	"cap":     {},
	"clear":   {},
	"copy":    {},
	"delete":  {},
	"len":     {},
	"make":    {},
	"max":     {},
	"min":     {},
	"panic":   {},
	"print":   {},
	"println": {},
}

// goTypes maps one of go's builtin type names to the conversion it applies,
// over a value already reduced to one of php's own scalars.
//
// A pair this table will not decide reports false and reaches the binding,
// which reduces the value through phpval first. A string becoming a number has
// one definition, phpval holds it, and this package cannot import phpval.
var goTypes = map[string]func(any) (any, bool){
	"any":     func(v any) (any, bool) { return v, true },
	"bool":    goBool,
	"string":  goString,
	"int":     goSigned(64),
	"int64":   goSigned(64),
	"float64": goFloat(64),
	"int8":    goNarrow("int8"),
	"int16":   goNarrow("int16"),
	"int32":   goNarrow("int32"),
	"rune":    goNarrow("rune"),
	"uint":    goNarrow("uint"),
	"uint8":   goNarrow("uint8"),
	"uint16":  goNarrow("uint16"),
	"uint32":  goNarrow("uint32"),
	"uint64":  goNarrow("uint64"),
	"byte":    goNarrow("byte"),
	"float32": goNarrow("float32"),
}

// goNarrow is the conversion of a sized type: it answers the value of that
// width, typed, which keeps the type a script wrote on the value it holds.
// `int`, `int64` and `float64` have no entry here: they name the widths php
// carries, and `int64(5)` is php's 5.
func goNarrow(name string) func(any) (any, bool) {
	w, ok := GoWidthNamed(name)
	if !ok {
		return func(any) (any, bool) { return nil, false }
	}
	return func(v any) (any, bool) {
		if w.Float {
			f, ok := goFloat64(v)
			if !ok {
				return nil, false
			}
			return w.Narrow(f), true
		}
		if _, isInt := v.(int64); !isInt {
			return nil, false
		}
		return w.Narrow(v), true
	}
}

// GoBuiltin reports whether name is a go builtin the import binds unqualified.
func GoBuiltin(name string) bool {
	if _, ok := goBuiltins[name]; ok {
		return true
	}
	return GoType(name)
}

// GoBuiltinNames returns every name GoBuiltin answers for, sorted.
func GoBuiltinNames() []string {
	names := make([]string, 0, len(goBuiltins)+len(goTypes))
	names = append(names, slices.Collect(maps.Keys(goBuiltins))...)
	names = append(names, slices.Collect(maps.Keys(goTypes))...)
	slices.Sort(names)
	return names
}

// GoType reports whether name is one of go's builtin types.
func GoType(name string) bool {
	_, ok := goTypes[name]
	return ok
}

// GoTypeNames returns go's builtin type names, sorted.
func GoTypeNames() []string {
	return slices.Sorted(maps.Keys(goTypes))
}

// GoFold converts v under the go type name and reports whether it did.
func GoFold(name string, v any) (any, bool) {
	convert, ok := goTypes[name]
	if !ok {
		return nil, false
	}
	return convert(v)
}

// goBool is bool's conversion, over a value php has already read for truth.
func goBool(v any) (any, bool) {
	b, ok := v.(bool)
	return b, ok
}

// goString is string's conversion, over a value php has already rendered.
func goString(v any) (any, bool) {
	s, ok := v.(string)
	return s, ok
}

// goSigned is the conversion of php's own integer width, which `int` and
// `int64` both name: php's int is 64 bits wide and the value passes through.
// A float or a numeric string reaches the binding, which reduces it through
// phpval before converting.
func goSigned(bits int) func(any) (any, bool) {
	return func(v any) (any, bool) {
		n, ok := v.(int64)
		if !ok {
			return nil, false
		}
		shift := 64 - bits
		return n << shift >> shift, true
	}
}

// goFloat is the conversion of php's own float width, which `float64` names.
func goFloat(bits int) func(any) (any, bool) {
	return func(v any) (any, bool) {
		f, ok := goFloat64(v)
		if !ok {
			return nil, false
		}
		if bits == 32 {
			return float64(float32(f)), true
		}
		return f, true
	}
}

// goFloat64 reads the values a float conversion takes: php's float, and its
// int, which widens exactly as go's conversion does.
func goFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int64:
		return float64(x), true
	}
	return 0, false
}
