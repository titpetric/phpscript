package phpval

import (
	"math"
	"testing"
)

// TestOverflowAddInt covers the carry detection the float fallback reads. A
// sum that leaves the int64 range reports false and a zero, so a caller never
// has a wrapped negative to tell apart from a real one.
func TestOverflowAddInt(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		x, y int64
		want int64
		ok   bool
	}{
		{name: "zero", x: 0, y: 0, want: 0, ok: true},
		{name: "positive", x: 2, y: 3, want: 5, ok: true},
		{name: "mixed signs", x: 5, y: -3, want: 2, ok: true},
		{name: "to the maximum", x: math.MaxInt64 - 1, y: 1, want: math.MaxInt64, ok: true},
		{name: "to the minimum", x: math.MinInt64 + 1, y: -1, want: math.MinInt64, ok: true},
		{name: "past the maximum", x: math.MaxInt64, y: 1},
		{name: "past the minimum", x: math.MinInt64, y: -1},
		{name: "two maxima", x: math.MaxInt64, y: math.MaxInt64},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := OverflowAddInt(tt.x, tt.y)
			if got != tt.want || ok != tt.ok {
				t.Errorf("OverflowAddInt(%d, %d) = %d, %t, want %d, %t", tt.x, tt.y, got, ok, tt.want, tt.ok)
			}
		})
	}
}

// TestOverflowSubInt covers the borrow detection. Subtracting a negative is
// the direction that overflows upward, so the cases carry both signs of y.
func TestOverflowSubInt(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		x, y int64
		want int64
		ok   bool
	}{
		{name: "zero", x: 0, y: 0, want: 0, ok: true},
		{name: "positive", x: 5, y: 3, want: 2, ok: true},
		{name: "below zero", x: 3, y: 5, want: -2, ok: true},
		{name: "subtracting a negative", x: 3, y: -5, want: 8, ok: true},
		{name: "to the minimum", x: math.MinInt64 + 1, y: 1, want: math.MinInt64, ok: true},
		{name: "past the minimum", x: math.MinInt64, y: 1},
		{name: "past the maximum", x: math.MaxInt64, y: -1},
		{name: "maximum less minimum", x: math.MaxInt64, y: math.MinInt64},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := OverflowSubInt(tt.x, tt.y)
			if got != tt.want || ok != tt.ok {
				t.Errorf("OverflowSubInt(%d, %d) = %d, %t, want %d, %t", tt.x, tt.y, got, ok, tt.want, tt.ok)
			}
		})
	}
}

// TestOverflowMulInt covers the division check and the one case it misses on
// its own: MinInt64 * -1 divides back cleanly and still has no int64 form,
// because the range holds no positive counterpart to MinInt64.
func TestOverflowMulInt(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		x, y int64
		want int64
		ok   bool
	}{
		{name: "zero on the left", x: 0, y: math.MaxInt64, want: 0, ok: true},
		{name: "zero on the right", x: math.MaxInt64, y: 0, want: 0, ok: true},
		{name: "positive", x: 6, y: 7, want: 42, ok: true},
		{name: "one negative", x: -6, y: 7, want: -42, ok: true},
		{name: "two negatives", x: -6, y: -7, want: 42, ok: true},
		{name: "identity at the maximum", x: math.MaxInt64, y: 1, want: math.MaxInt64, ok: true},
		{name: "negated maximum", x: math.MaxInt64, y: -1, want: -math.MaxInt64, ok: true},
		{name: "doubled maximum", x: math.MaxInt64, y: 2},
		{name: "negated minimum", x: -1, y: math.MinInt64},
		{name: "doubled minimum", x: math.MinInt64, y: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := OverflowMulInt(tt.x, tt.y)
			if got != tt.want || ok != tt.ok {
				t.Errorf("OverflowMulInt(%d, %d) = %d, %t, want %d, %t", tt.x, tt.y, got, ok, tt.want, tt.ok)
			}
		})
	}
}

// TestAddInt covers PHP's + on two ints. php 8.5 prints
// float(9.223372036854776E+18) for PHP_INT_MAX + 1, so a sum that leaves the
// range changes type and the Go wrap is never visible to a script.
func TestAddInt(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		x, y int64
		want any
	}{
		{name: "int stays int", x: 2, y: 3, want: int64(5)},
		{name: "negative sum", x: 2, y: -5, want: int64(-3)},
		{name: "at the maximum", x: math.MaxInt64 - 1, y: 1, want: int64(math.MaxInt64)},
		{name: "past the maximum", x: math.MaxInt64, y: 1, want: float64(math.MaxInt64) + 1},
		{name: "past the minimum", x: math.MinInt64, y: -1, want: float64(math.MinInt64) - 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := AddInt(tt.x, tt.y); got != tt.want {
				t.Errorf("AddInt(%d, %d) = %#v, want %#v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

// TestSubInt covers PHP's - on two ints. php 8.5 prints
// float(-9.223372036854776E+18) for PHP_INT_MIN - 1.
func TestSubInt(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		x, y int64
		want any
	}{
		{name: "int stays int", x: 5, y: 3, want: int64(2)},
		{name: "below zero", x: 3, y: 5, want: int64(-2)},
		{name: "past the minimum", x: math.MinInt64, y: 1, want: float64(math.MinInt64) - 1},
		{name: "past the maximum", x: math.MaxInt64, y: -1, want: float64(math.MaxInt64) + 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := SubInt(tt.x, tt.y); got != tt.want {
				t.Errorf("SubInt(%d, %d) = %#v, want %#v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

// TestMulInt covers PHP's * on two ints. php 8.5 prints
// float(1.8446744073709552E+19) for PHP_INT_MAX * 2 and
// float(9.223372036854776E+18) for PHP_INT_MIN * -1.
func TestMulInt(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		x, y int64
		want any
	}{
		{name: "int stays int", x: 6, y: 7, want: int64(42)},
		{name: "by zero", x: math.MaxInt64, y: 0, want: int64(0)},
		{name: "doubled maximum", x: math.MaxInt64, y: 2, want: float64(math.MaxInt64) * 2},
		{name: "negated minimum", x: math.MinInt64, y: -1, want: float64(math.MinInt64) * -1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := MulInt(tt.x, tt.y); got != tt.want {
				t.Errorf("MulInt(%d, %d) = %#v, want %#v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

// TestDivInt covers PHP's / on two ints, where the operator is true division
// and no integer one: php 8.5 prints float(3.5) for 7/2 and int(3) for 6/2,
// so an even division keeps the int type and an uneven one changes it.
//
// Division by zero returns int 0 here. php 8.5 throws DivisionByZeroError; the
// warning and the throw are the interpreter's, and this function is the
// arithmetic alone.
func TestDivInt(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		x, y int64
		want any
	}{
		{name: "even division stays int", x: 6, y: 2, want: int64(3)},
		{name: "uneven division is float", x: 7, y: 2, want: 3.5},
		{name: "negative even", x: -6, y: 2, want: int64(-3)},
		{name: "negative uneven", x: -7, y: 2, want: -3.5},
		{name: "by one", x: 7, y: 1, want: int64(7)},
		{name: "by zero", x: 7, y: 0, want: int64(0)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := DivInt(tt.x, tt.y); got != tt.want {
				t.Errorf("DivInt(%d, %d) = %#v, want %#v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

// TestModInt covers PHP's %, which takes the sign of the left operand: php 8.5
// prints int(-1) for -7%3 and int(1) for 7%-3. Go's % has the same rule, so
// the operator passes through.
//
// Modulo by zero returns int 0, matching the interpreter's phpArith. php 8.5
// throws DivisionByZeroError from the operator itself.
func TestModInt(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		x, y int64
		want any
	}{
		{name: "remainder", x: 7, y: 3, want: int64(1)},
		{name: "no remainder", x: 6, y: 3, want: int64(0)},
		{name: "negative left operand", x: -7, y: 3, want: int64(-1)},
		{name: "negative right operand", x: 7, y: -3, want: int64(1)},
		{name: "by zero", x: 7, y: 0, want: int64(0)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ModInt(tt.x, tt.y); got != tt.want {
				t.Errorf("ModInt(%d, %d) = %#v, want %#v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}
