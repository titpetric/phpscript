package model

import (
	"math"
)

// A typed go value is a value a script holds under one of go's sized numeric
// types: `int8(300)` is an int8 holding 44 and not php's int holding 44, and
// var_dump reads it back as `int8(44)`. php's own int, float, string and bool
// carry no such type, and neither do `int`, `int64` and `float64`, which name
// the widths php already has.
//
// The type travels with the value: through an array, a go slice, a binding
// and a return. Arithmetic keeps it and wraps at the width, as go's does and
// php's has no notion of; the operators read GoWidth to compute at one. See
// docs/reference/extensions/go-runtime.md.

// GoWidth is the shape of a typed go numeric: the spelling a script wrote, the
// bits it holds, whether it reads signed, and whether it reads as a float.
type GoWidth struct {
	Name   string
	Bits   int
	Signed bool
	Float  bool
}

// Zero is the value of the width holding no value, the one a slice of it
// reads back before anything writes to it.
func (w GoWidth) Zero() any {
	if w.Float {
		return w.Narrow(float64(0))
	}
	return w.Narrow(int64(0))
}

// goWidths is every sized numeric a conversion returns, by the name a script
// writes. `rune` and `byte` are go's own aliases and carry the width they
// alias: go prints a rune as int32, and so does this.
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

// GoWidthNamed returns the width a go type name carries, if it carries one.
func GoWidthNamed(name string) (GoWidth, bool) {
	w, ok := goWidths[name]
	return w, ok
}

// GoWidthOf returns the width a value carries. php's int, float, string and
// bool carry none, and neither does int64: php's int is 64 bits wide and
// `int64(5)` is php's 5.
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

// GoWiden reads a typed numeric as php reads a number: an integer width
// answers an int64, a float width a float64, and an unsigned value above
// PHP_INT_MAX answers a float64, which is the only php number that holds it.
func GoWiden(v any) (any, bool) {
	switch x := v.(type) {
	case int8:
		return int64(x), true
	case int16:
		return int64(x), true
	case int32:
		return int64(x), true
	case uint:
		return widenUnsigned(uint64(x)), true
	case uint8:
		return int64(x), true
	case uint16:
		return int64(x), true
	case uint32:
		return int64(x), true
	case uint64:
		return widenUnsigned(x), true
	case float32:
		return float64(x), true
	}
	return nil, false
}

// GoInt reads a typed integer value as the 64 bits it holds, the bits an
// operation at its width computes over. An unsigned value past PHP_INT_MAX
// keeps its bits here, where GoWiden reaches the float php would hold it in.
func GoInt(v any) (int64, bool) {
	switch x := v.(type) {
	case int8:
		return int64(x), true
	case int16:
		return int64(x), true
	case int32:
		return int64(x), true
	case uint:
		return int64(x), true
	case uint8:
		return int64(x), true
	case uint16:
		return int64(x), true
	case uint32:
		return int64(x), true
	case uint64:
		return int64(x), true
	}
	return 0, false
}

// GoClipInt reads a typed integer as php's int, clipping an unsigned value
// past PHP_INT_MAX at PHP_INT_MAX: go's conversion answers -1 there and the
// float php holds it in reduces to 0.
func GoClipInt(v any) (int64, bool) {
	w, typed := GoWidthOf(v)
	if !typed || w.Float {
		return 0, false
	}
	bits, _ := GoInt(v)
	if !w.Signed && bits < 0 {
		return math.MaxInt64, true
	}
	return bits, true
}

// GoUint is GoInt over unsigned bits, the domain an unsigned width computes in.
func GoUint(v any) (uint64, bool) {
	n, ok := GoInt(v)
	return uint64(n), ok
}

// GoStep applies ++ or -- at a typed value's width, and reports whether the
// value carried one. The step wraps where go's does: int8(127) steps up to
// int8(-128), where php's int reaches a float at its own limit.
func GoStep(v any, delta int64) (any, bool) {
	w, typed := GoWidthOf(v)
	if !typed {
		return nil, false
	}
	if w.Float {
		return w.Narrow(toFloat(v) + float64(delta)), true
	}
	return w.Narrow(toInt(v) + delta), true
}

// widenUnsigned reads an unsigned value as the php number that holds it.
func widenUnsigned(u uint64) any {
	if u > math.MaxInt64 {
		return float64(u)
	}
	return int64(u)
}

// Narrow builds the value of the width out of a php number, wrapping as go's
// conversion wraps: int8(300) keeps the low bits and sign-extends them, and
// uint8(-1) is 255.
//
// An integer width takes an integer. A float reduces to one through
// phpval.Int, where php's own float-to-int reduction lives, and this package
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

// toInt reads the integer behind a php integer or a typed value, for a
// conversion that has already reduced the value to one.
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
	}
	// The bits, and not the php number: narrowing uint64(-1) to a uint8 reads
	// the low byte of 18446744073709551615, where the float php holds that
	// value in has no low byte left.
	if bits, ok := GoInt(v); ok {
		return bits
	}
	if widened, ok := GoWiden(v); ok {
		return toInt(widened)
	}
	return 0
}

// toFloat is toInt for a float width.
func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case int:
		return float64(x)
	}
	if widened, ok := GoWiden(v); ok {
		return toFloat(widened)
	}
	return 0
}
