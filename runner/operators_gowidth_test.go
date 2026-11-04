package runner

import (
	"errors"
	"math"
	"testing"
)

// TestGoArith covers arithmetic at a typed value's width. Two values of one
// width answer that width and wrap in it, a typed value against one of php's
// numbers answers the typed width, and two different widths answer a
// TypeError, as go refuses the pair outright.
func TestGoArith(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		op   string
		a, b any
		want any
	}{
		{"signed addition wraps", "+", int8(100), int8(100), int8(-56)},
		{"a php number converts", "+", int8(100), int64(1), int8(101)},
		{"the left operand may be php's", "*", int64(2), int8(3), int8(6)},
		{"unsigned subtraction wraps", "-", uint8(0), int64(1), uint8(255)},
		{"unsigned at the widest", "-", uint64(0), int64(1), uint64(math.MaxUint64)},
		{"division truncates", "/", int8(7), int8(2), int8(3)},
		{"division by zero is the zero value", "/", int8(7), int8(0), int8(0)},
		{"modulo keeps the width", "%", int8(7), int8(2), int8(1)},
		{"a float width computes in float", "/", float32(1), float32(3), float32(1) / float32(3)},
		{"the exponent wraps at the width", "**", int8(2), int8(10), int8(0)},
	} {
		got, typed, err := goArith(tc.op, tc.a, tc.b)
		if !typed {
			t.Errorf("%s: goArith(%q) read no width", tc.name, tc.op)
			continue
		}
		if err != nil {
			t.Errorf("%s: goArith(%q): %v", tc.name, tc.op, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: goArith(%q) = %#v, want %#v", tc.name, tc.op, got, tc.want)
		}
	}
}

// TestGoArithOperands covers the two operand pairs no width computes over.
// Two of php's own numbers keep php's arithmetic and its overflow to float,
// and two different widths answer the class php words a refusal between
// operand types under.
func TestGoArithOperands(t *testing.T) {
	t.Parallel()
	if _, typed, err := goArith("+", int64(1), 0.5); typed || err != nil {
		t.Errorf("goArith on two php numbers reported a width (err %v)", err)
	}

	_, typed, err := goArith("+", int8(1), int16(1))
	if !typed {
		t.Fatal("goArith(int8, int16) read no width")
	}
	var operand *OperandError
	if !errors.As(err, &operand) {
		t.Fatalf("goArith(int8, int16) = %v, want an OperandError", err)
	}
	if want := "Unsupported operand types: int8 + int16"; operand.Message != want {
		t.Errorf("the message is %q, want %q", operand.Message, want)
	}
}

// TestGoBitwise covers & | ^ << >> at a width. The result narrows to the
// width and a shift past it leaves zero, as go's does, and a right shift
// reads the width's own sign. A negative count has no answer and raises the
// class php raises for it.
func TestGoBitwise(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		op   string
		a, b any
		want any
	}{
		{"and", "&", uint8(12), uint8(10), uint8(8)},
		{"or", "|", uint8(12), uint8(10), uint8(14)},
		{"xor", "^", uint8(12), uint8(10), uint8(6)},
		{"a shift out of the width", "<<", int32(1), int64(31), int32(math.MinInt32)},
		{"a shift past the width", "<<", int8(1), int64(64), int8(0)},
		{"an unsigned shift reads zeroes in", ">>", uint8(255), int64(4), uint8(15)},
		{"a signed shift keeps the sign", ">>", int8(-8), int64(1), int8(-4)},
		{"a signed shift past the width", ">>", int8(-8), int64(64), int8(-1)},
	} {
		got, typed, err := goBitwise(tc.op, tc.a, tc.b)
		if !typed {
			t.Errorf("%s: goBitwise(%q) read no width", tc.name, tc.op)
			continue
		}
		if err != nil {
			t.Errorf("%s: goBitwise(%q): %v", tc.name, tc.op, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s: goBitwise(%q) = %#v, want %#v", tc.name, tc.op, got, tc.want)
		}
	}

	if _, _, err := goBitwise("<<", int8(1), int64(-1)); err == nil {
		t.Error("goBitwise shifted by a negative count without an error")
	}
	if _, _, err := goBitwise("&", float32(1), float32(1)); err == nil {
		t.Error("goBitwise read bits off a float width without an error")
	}
}

// TestGoUnary covers `-` and `~` at a width. Both wrap in it: negating the
// smallest int8 answers itself, and inverting the bits of an unsigned zero
// answers the width's largest value where php's int answers -1.
func TestGoUnary(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value any
		want  any
	}{
		{int8(-128), int8(-128)},
		{int8(5), int8(-5)},
		{uint8(1), uint8(255)},
		{float32(0.5), float32(-0.5)},
	} {
		got, typed := goNegate(tc.value)
		if !typed {
			t.Errorf("goNegate(%#v) read no width", tc.value)
			continue
		}
		if got != tc.want {
			t.Errorf("goNegate(%#v) = %#v, want %#v", tc.value, got, tc.want)
		}
	}
	if _, typed := goNegate(int64(1)); typed {
		t.Error("goNegate(int64(1)) reported a width")
	}

	got, typed := goBitNot(uint8(0))
	if !typed {
		t.Fatal("goBitNot(uint8(0)) read no width")
	}
	if want := uint8(255); got != want {
		t.Errorf("goBitNot(uint8(0)) = %#v, want %#v", got, want)
	}
	if _, typed := goBitNot(float32(1)); typed {
		t.Error("goBitNot(float32(1)) reported a width; a float carries no bits")
	}
}
