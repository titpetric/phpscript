package model

import (
	"fmt"
)

// A typed go value is a value a script holds under one of go's sized numeric
// types: `int8(300)` is an int8 holding 44 and not php's int holding 44, and
// var_dump reads it back as `int8(44)`. php's own int, float, string and bool
// hold no such type, and `int`, `int64` and `float64` hold none either: they
// name the widths php already has.
//
// The type travels with the value: through an array, a go slice, a binding
// and a return. Arithmetic keeps it and wraps at the width, as go's does and
// php's has no notion of; modelutil reads GoWidth to compute at one. See
// docs/reference/extensions/go-runtime.md.

// GoWidth is the shape of a typed go numeric: the spelling a script wrote, the
// bits it holds, whether it reads signed, and whether it reads as a float.
type GoWidth struct {
	Name   string
	Bits   int
	Signed bool
	Float  bool
}

// goWidths is every sized numeric a conversion returns, by the name a script
// writes. `rune` and `byte` are go's own aliases and take the width they
// alias: go prints a rune as int32. So does this.
var goWidths = map[string]GoWidth{
	"int8":    {Name: "int8", Bits: 8, Signed: true},
	"int16":   {Name: "int16", Bits: 16, Signed: true},
	"int32":   {Name: "int32", Bits: 32, Signed: true},
	"rune":    {Name: "int32", Bits: 32, Signed: true},
	"uint":    {Name: "uint", Bits: 64},
	"uint8":   {Name: "uint8", Bits: 8},
	"byte":    {Name: "uint8", Bits: 8},
	"uint16":  {Name: "uint16", Bits: 16},
	"uint32":  {Name: "uint32", Bits: 32},
	"uint64":  {Name: "uint64", Bits: 64},
	"float32": {Name: "float32", Bits: 32, Float: true},
}

// GoWidthNamed returns the width goWidths holds for a go type name.
func GoWidthNamed(name string) (GoWidth, bool) {
	w, ok := goWidths[name]
	return w, ok
}

// GoWidthOf returns the width a value holds. php's int, float, string, bool
// and int64 hold none: php's int is 64 bits wide and `int64(5)` is php's 5.
func GoWidthOf(v any) (GoWidth, bool) {
	switch v.(type) {
	case int8:
		return goWidths["int8"], true
	case int16:
		return goWidths["int16"], true
	case int32:
		return goWidths["int32"], true
	case uint:
		return goWidths["uint"], true
	case uint8:
		return goWidths["uint8"], true
	case uint16:
		return goWidths["uint16"], true
	case uint32:
		return goWidths["uint32"], true
	case uint64:
		return goWidths["uint64"], true
	case float32:
		return goWidths["float32"], true
	}
	return GoWidth{}, false
}

// Valid reports a width naming a spelling and a size go declares, and an error
// for a zero value or one a caller assembled by hand.
func (w GoWidth) Valid() error {
	known, ok := goWidths[w.Name]
	if !ok {
		return fmt.Errorf("go width %q names no go numeric type", w.Name)
	}
	if known != w {
		return fmt.Errorf("go width %q declares %d bits, signed %v, float %v; go declares %d, %v, %v",
			w.Name, w.Bits, w.Signed, w.Float, known.Bits, known.Signed, known.Float)
	}
	return nil
}

// Zero is the value of the width holding no value, the one a slice of it
// reads back before anything writes to it.
func (w GoWidth) Zero() any {
	if w.Float {
		return w.Narrow(float64(0))
	}
	return w.Narrow(int64(0))
}

// Narrow builds the value of the width out of a php number, wrapping as go's
// conversion wraps: int8(300) keeps the low bits and sign-extends them, and
// uint8(-1) is 255.
//
// An integer width takes an integer. A float reduces to one through
// phpval.Int, where php's own float-to-int reduction lives. This package
// holds no second copy of it.
func (w GoWidth) Narrow(v any) any {
	if w.Float {
		return float32(toFloat(v))
	}
	n := toInt(v)
	if w.Signed {
		switch w.Bits {
		case 8:
			return int8(n)
		case 16:
			return int16(n)
		case 32:
			return int32(n)
		}
		return n
	}
	switch w.Bits {
	case 8:
		return uint8(n)
	case 16:
		return uint16(n)
	case 32:
		return uint32(n)
	}
	if w.Name == "uint" {
		return uint(n)
	}
	return uint64(n)
}

// The bits, and not the php number: narrowing uint64(-1) to a uint8 reads the
// low byte of 18446744073709551615, which the float php holds it in rounds off.
func toInt(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	case int8:
		return int64(x)
	case int16:
		return int64(x)
	case int32:
		return int64(x)
	case uint:
		return int64(x)
	case uint8:
		return int64(x)
	case uint16:
		return int64(x)
	case uint32:
		return int64(x)
	case uint64:
		return int64(x)
	case float32:
		return int64(float64(x))
	case float64:
		return int64(x)
	}
	return 0
}

// toFloat is toInt for a float width.
func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int64:
		return float64(x)
	case int:
		return float64(x)
	case int8:
		return float64(x)
	case int16:
		return float64(x)
	case int32:
		return float64(x)
	case uint:
		return float64(x)
	case uint8:
		return float64(x)
	case uint16:
		return float64(x)
	case uint32:
		return float64(x)
	case uint64:
		return float64(x)
	}
	return 0
}
