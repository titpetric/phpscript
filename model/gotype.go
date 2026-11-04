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
// unqualified. php's own copy, min, max and print are not among them: a file
// that imported the go runtime still calls php's when it writes one, and
// `Go\copy` names the other.
var goBuiltins = map[string]struct{}{
	"append":  {},
	"cap":     {},
	"clear":   {},
	"delete":  {},
	"len":     {},
	"make":    {},
	"panic":   {},
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
	"int8":    goSigned(8),
	"int16":   goSigned(16),
	"int32":   goSigned(32),
	"int64":   goSigned(64),
	"rune":    goSigned(32),
	"uint":    goUnsigned(64),
	"uint8":   goUnsigned(8),
	"uint16":  goUnsigned(16),
	"uint32":  goUnsigned(32),
	"uint64":  goUnsigned(64),
	"byte":    goUnsigned(8),
	"float32": goFloat(32),
	"float64": goFloat(64),
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

// goSigned is the conversion of a signed integer type at the given width.
//
// The carrier is php's int, which is 64 bits wide: a narrower type keeps the
// low bits of the value and sign-extends them, as go's conversion does, and
// int8(300) is 44 in both. The shift pair is that truncation.
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

// goUnsigned is the same conversion for an unsigned type, where the low bits
// stay as they are and the carrier stays signed: a uint64 over PHP_INT_MAX
// reads back negative, as every such value does in php.
func goUnsigned(bits int) func(any) (any, bool) {
	return func(v any) (any, bool) {
		n, ok := v.(int64)
		if !ok {
			return nil, false
		}
		if bits == 64 {
			return n, true
		}
		return n & (1<<bits - 1), true
	}
}

// goFloat is the conversion of a float type. 32 is the width that loses
// precision: the value reads back as the float64 nearest the float32, php
// carrying one float type and that one wide.
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
