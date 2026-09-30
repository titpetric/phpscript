package phpval

import (
	"math"
)

// This file holds PHP's int64 arithmetic, shared by the interpreter's
// phpArith and the flat VM's specialised integer opcodes so the overflow and
// division rules have one home.

// The Overflow* trio performs int64 arithmetic and reports false rather than
// wrapping, so a caller can fall back to float the way PHP does.

// OverflowAddInt adds x and y.
func OverflowAddInt(x, y int64) (int64, bool) {
	z := x + y
	if (y > 0 && z < x) || (y < 0 && z > x) {
		return 0, false
	}
	return z, true
}

// OverflowSubInt subtracts y from x.
func OverflowSubInt(x, y int64) (int64, bool) {
	z := x - y
	if (y < 0 && z < x) || (y > 0 && z > x) {
		return 0, false
	}
	return z, true
}

// OverflowMulInt multiplies x and y. MinInt64 * -1 is an overflow too: it has
// no positive counterpart.
func OverflowMulInt(x, y int64) (int64, bool) {
	z := x * y
	if x != 0 && (z/x != y || (x == -1 && y == math.MinInt64)) {
		return 0, false
	}
	return z, true
}

// AddInt, SubInt and MulInt add PHP semantics on top: integer arithmetic that
// overflows becomes float, so PHP_INT_MAX + 1 is 9.2233720368548E+18 rather
// than a wrapped negative.

// AddInt is PHP's + on two ints.
func AddInt(x, y int64) any {
	if z, ok := OverflowAddInt(x, y); ok {
		return z
	}
	return float64(x) + float64(y)
}

// SubInt is PHP's - on two ints.
func SubInt(x, y int64) any {
	if z, ok := OverflowSubInt(x, y); ok {
		return z
	}
	return float64(x) - float64(y)
}

// MulInt is PHP's * on two ints.
func MulInt(x, y int64) any {
	if z, ok := OverflowMulInt(x, y); ok {
		return z
	}
	return float64(x) * float64(y)
}

// DivInt divides the way PHP's / does on two ints: division by zero yields
// int 0 (the interpreter reports the warning elsewhere), a division that does
// not come out even is float, and an even one stays int.
func DivInt(x, y int64) any {
	if y == 0 {
		return int64(0)
	}
	if x%y != 0 {
		return float64(x) / float64(y)
	}
	return x / y
}

// ModInt is PHP's %, already int-cast by the caller; modulo by zero yields
// int 0 to match the interpreter's phpArith.
func ModInt(x, y int64) any {
	if y == 0 {
		return int64(0)
	}
	return x % y
}
