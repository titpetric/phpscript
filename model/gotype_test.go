package model_test

import (
	"math"
	"slices"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestGoBuiltin covers the two name tables the import binds from: go's
// builtins and go's types, which answer one predicate because a script calls
// both the same way.
func TestGoBuiltin(t *testing.T) {
	for _, name := range []string{"make", "append", "len", "cap", "clear", "delete", "panic", "println", "int64", "any"} {
		if !model.GoBuiltin(name) {
			t.Errorf("%s does not read as a builtin", name)
		}
	}
	// The four that shadow a php name the import binds too: an import is the
	// more specific name and `\min` is how a file reaches php's.
	for _, name := range []string{"copy", "min", "max", "print"} {
		if !model.GoBuiltin(name) {
			t.Errorf("%s does not read as a builtin", name)
		}
	}
	// A name go has no builtin for stays php's, imported or not.
	for _, name := range []string{"new", "strlen", "array_keys", ""} {
		if model.GoBuiltin(name) {
			t.Errorf("%s reads as a builtin the import binds", name)
		}
	}
	if !model.GoType("float32") || model.GoType("Thing") {
		t.Error("GoType does not answer for go's types alone")
	}
	names := model.GoBuiltinNames()
	if !slices.IsSorted(names) {
		t.Errorf("the names are not sorted: %v", names)
	}
	if !slices.Contains(names, "make") || !slices.Contains(names, "uint8") {
		t.Errorf("the names cover one table and not both: %v", names)
	}
	if got, want := len(model.GoTypeNames()), 17; got != want {
		t.Errorf("GoTypeNames returns %d names, want %d", got, want)
	}
}

// TestGoFold covers the conversion a type name applies to a value php has
// already reduced to one of its own scalars, and the pairs it refuses: a
// string under a numeric type is phpval's to read, not this package's.
func TestGoFold(t *testing.T) {
	for _, test := range []struct {
		name string
		in   any
		want any
	}{
		{"any", "anything", "anything"},
		{"any", nil, nil},
		{"bool", true, true},
		{"string", "text", "text"},
		{"int", int64(7), int64(7)},
		{"int64", int64(0), int64(0)},
		{"int64", int64(math.MinInt64), int64(math.MinInt64)},
		// A sized type answers a value of that type, and var_dump reads it
		// back as int8(44) while the arithmetic wraps at the width.
		{"int8", int64(300), int8(44)},
		{"int8", int64(-1), int8(-1)},
		{"int16", int64(70000), int16(4464)},
		{"int32", int64(math.MaxInt32) + 1, int32(math.MinInt32)},
		{"rune", int64(65), int32(65)},
		{"uint8", int64(300), uint8(44)},
		{"uint8", int64(-1), uint8(255)},
		{"byte", int64(255), uint8(255)},
		{"uint16", int64(-1), uint16(65535)},
		{"uint32", int64(-1), uint32(4294967295)},
		{"uint64", int64(-1), uint64(math.MaxUint64)},
		{"uint", int64(-1), uint(math.MaxUint64)},
		{"float64", int64(3), float64(3)},
		{"float64", 1.5, 1.5},
		{"float32", 0.1, float32(0.1)},
		{"float32", int64(2), float32(2)},
	} {
		got, ok := model.GoFold(test.name, test.in)
		if !ok {
			t.Errorf("%s(%v) folded nothing", test.name, test.in)
			continue
		}
		if got != test.want {
			t.Errorf("%s(%v) = %#v, want %#v", test.name, test.in, got, test.want)
		}
	}

	for _, test := range []struct {
		name string
		in   any
	}{
		{"int64", "12"},
		{"int64", 1.9},
		{"int64", true},
		{"uint8", nil},
		{"bool", int64(1)},
		{"string", int64(1)},
		{"float64", "1.5"},
		{"Thing", int64(1)},
	} {
		if _, ok := model.GoFold(test.name, test.in); ok {
			t.Errorf("%s(%#v) folded a pair phpval decides", test.name, test.in)
		}
	}
}
