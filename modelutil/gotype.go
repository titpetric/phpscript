package modelutil

import (
	"maps"
	"slices"

	"github.com/titpetric/phpscript/model"
)

// This file is the go runtime's name tables: the namespace a script imports to
// bind go's builtins, the names that import binds, and the conversion a
// builtin type name applies. The parser reads them to bind a name and fold a
// literal while it parses, the bindings read them to register and to convert.
// This file spells the list once.

// GoNamespace is the namespace a script imports to bind go's builtins.
const GoNamespace = `Runtime\Go`

// goBuiltins holds the names the go import binds unqualified, four of which shadow php.
var goBuiltins = map[string]struct{}{
	"append":  {},
	"cap":     {},
	"clear":   {},
	"copy":    {},
	"delete":  {},
	"getenv":  {},
	"len":     {},
	"make":    {},
	"max":     {},
	"min":     {},
	"numcpu":  {},
	"panic":   {},
	"print":   {},
	"println": {},
}

// goTypes maps a go type name to its conversion, false for a pair it declines.
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

// goNarrow converts through the sized type name and keeps the width it names.
func goNarrow(name string) func(any) (any, bool) {
	w, ok := model.GoWidthNamed(name)
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

// goSigned converts an int64 at php's own width and declines every other type.
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

// goFloat64 reads a php float or int as float64 and declines anything else.
func goFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int64:
		return float64(x), true
	}
	return 0, false
}
