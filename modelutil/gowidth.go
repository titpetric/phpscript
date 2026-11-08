package modelutil

import (
	"math"

	"github.com/titpetric/phpscript/model"
)

// The width descriptor and its own conversion are model.GoWidth's. These are
// the readers over an `any`: a value arrives from a binding, an array or a
// return holding whatever type the call put in it. The arithmetic needs the
// bits before it knows the width. See
// docs/reference/extensions/go-runtime.md.

// GoWiden reads a typed numeric as php reads a number: an integer width answers
// an int64, a float width a float64, an unsigned above PHP_INT_MAX a float64.
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

// GoInt reads a typed integer as the 64 bits it holds. An unsigned value past
// PHP_INT_MAX keeps its bits, where GoWiden reaches the float php holds it in.
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
	w, typed := model.GoWidthOf(v)
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
// value held one. The step wraps where go's does: int8(127) steps up to
// int8(-128), where php's int reaches a float at its own limit.
func GoStep(v any, delta int64) (any, bool) {
	w, typed := model.GoWidthOf(v)
	if !typed {
		return nil, false
	}
	if w.Float {
		return w.Narrow(toFloat(v) + float64(delta)), true
	}
	return w.Narrow(toInt(v) + delta), true
}

// A value past PHP_INT_MAX reaches php as the float that holds it.
func widenUnsigned(u uint64) any {
	if u > math.MaxInt64 {
		return float64(u)
	}
	return int64(u)
}

func toInt(v any) int64 {
	if bits, ok := GoInt(v); ok {
		return bits
	}
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case float32:
		return int64(float64(x))
	case bool:
		if x {
			return 1
		}
	}
	return 0
}

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
	}
	if widened, ok := GoWiden(v); ok {
		return toFloat(widened)
	}
	return 0
}
