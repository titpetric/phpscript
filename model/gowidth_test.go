package model_test

import (
	"math"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestGoWidthNamed covers the width table. A sized name answers its bits and
// its sign, go's two aliases answer the width they alias, and a name php
// already carries - int, int64, float64 - answers nothing, because `int64(5)`
// is php's 5 and carries no width.
func TestGoWidthNamed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		want   string
		bits   int
		signed bool
		float  bool
	}{
		{name: "int8", want: "int8", bits: 8, signed: true},
		{name: "uint8", want: "uint8", bits: 8},
		{name: "byte", want: "uint8", bits: 8},
		{name: "rune", want: "int32", bits: 32, signed: true},
		{name: "uint64", want: "uint64", bits: 64},
		{name: "float32", want: "float32", bits: 32, float: true},
	} {
		w, ok := model.GoWidthNamed(tc.name)
		if !ok {
			t.Errorf("GoWidthNamed(%q) = false", tc.name)
			continue
		}
		if w.Name != tc.want || w.Bits != tc.bits || w.Signed != tc.signed || w.Float != tc.float {
			t.Errorf("GoWidthNamed(%q) = %+v, want %s/%d/%t/%t", tc.name, w, tc.want, tc.bits, tc.signed, tc.float)
		}
	}
	for _, name := range []string{"int", "int64", "float64", "string", "bool", "any", "Row"} {
		if w, ok := model.GoWidthNamed(name); ok {
			t.Errorf("GoWidthNamed(%q) = %+v, want no width", name, w)
		}
	}
}

// TestGoWidthOf covers the width a value carries. php's own types carry none,
// and int64 is among them: php's int is the 64 bit one.
func TestGoWidthOf(t *testing.T) {
	t.Parallel()
	for value, want := range map[any]string{
		int8(1):    "int8",
		int16(1):   "int16",
		int32(1):   "int32",
		uint(1):    "uint",
		uint8(1):   "uint8",
		uint16(1):  "uint16",
		uint32(1):  "uint32",
		uint64(1):  "uint64",
		float32(1): "float32",
	} {
		w, ok := model.GoWidthOf(value)
		if !ok {
			t.Errorf("GoWidthOf(%#v) = false", value)
			continue
		}
		if w.Name != want {
			t.Errorf("GoWidthOf(%#v) = %q, want %q", value, w.Name, want)
		}
	}
	for _, value := range []any{nil, int64(1), int(1), float64(1), "1", true, model.NewArray()} {
		if w, ok := model.GoWidthOf(value); ok {
			t.Errorf("GoWidthOf(%#v) = %q, want no width", value, w.Name)
		}
	}
}

// TestGoWiden covers reading a typed value as a php number. Every integer
// width answers php's int, a float width answers php's float, and an unsigned
// value above PHP_INT_MAX answers a float, which is the only php number that
// holds it.
func TestGoWiden(t *testing.T) {
	t.Parallel()
	for value, want := range map[any]any{
		int8(-5):               int64(-5),
		int16(-5):              int64(-5),
		int32(-5):              int64(-5),
		uint8(255):             int64(255),
		uint16(65535):          int64(65535),
		uint32(1):              int64(1),
		uint(7):                int64(7),
		uint64(math.MaxUint64): float64(math.MaxUint64),
		float32(0.5):           float64(0.5),
	} {
		got, ok := model.GoWiden(value)
		if !ok {
			t.Errorf("GoWiden(%#v) = false", value)
			continue
		}
		if got != want {
			t.Errorf("GoWiden(%#v) = %#v, want %#v", value, got, want)
		}
	}
	if got, ok := model.GoWiden(int64(1)); ok {
		t.Errorf("GoWiden(int64(1)) = %#v, want no widening", got)
	}
}

// TestGoInt covers the bits an operation computes over. An unsigned value past
// PHP_INT_MAX keeps its bits here, where GoWiden reaches the float php holds
// it in, and narrowing it back reads the low byte it still has.
func TestGoInt(t *testing.T) {
	t.Parallel()
	bits, ok := model.GoInt(uint64(math.MaxUint64))
	if !ok {
		t.Fatal("GoInt(uint64 max) = false")
	}
	if bits != -1 {
		t.Errorf("GoInt(uint64 max) = %d, want -1", bits)
	}
	unsigned, ok := model.GoUint(uint64(math.MaxUint64))
	if !ok {
		t.Fatal("GoUint(uint64 max) = false")
	}
	if unsigned != math.MaxUint64 {
		t.Errorf("GoUint(uint64 max) = %d, want %d", unsigned, uint64(math.MaxUint64))
	}
	if _, ok := model.GoInt(float32(1)); ok {
		t.Error("GoInt(float32(1)) = true, want no integer bits")
	}
}

// TestGoClipInt covers the integer php reads a typed value as. An unsigned
// value past PHP_INT_MAX clips there, where the bits read -1 and the float php
// holds the value in reduces to 0. A float width answers no integer at all,
// the float domain holding the value php reads.
func TestGoClipInt(t *testing.T) {
	t.Parallel()
	for value, want := range map[any]int64{
		int8(-5):               -5,
		uint8(255):             255,
		uint64(math.MaxUint64): math.MaxInt64,
		uint(math.MaxUint64):   math.MaxInt64,
		uint64(7):              7,
	} {
		got, ok := model.GoClipInt(value)
		if !ok {
			t.Errorf("GoClipInt(%#v) = false", value)
			continue
		}
		if got != want {
			t.Errorf("GoClipInt(%#v) = %d, want %d", value, got, want)
		}
	}
	for _, value := range []any{float32(1), int64(1), "1", nil} {
		if got, ok := model.GoClipInt(value); ok {
			t.Errorf("GoClipInt(%#v) = %d, want php's own reading", value, got)
		}
	}
}

// TestGoWidthNarrow covers the conversion. A value wider than the width keeps
// its low bits and sign-extends them, as go's conversion does, and a negative
// value into an unsigned width wraps: uint8(-1) is 255.
func TestGoWidthNarrow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		width string
		value any
		want  any
	}{
		{"int8", int64(300), int8(44)},
		{"int8", int64(-129), int8(127)},
		{"int16", int64(70000), int16(4464)},
		{"int32", int64(math.MaxInt32) + 1, int32(math.MinInt32)},
		{"uint8", int64(-1), uint8(255)},
		{"uint16", int64(-1), uint16(65535)},
		{"uint32", int64(-1), uint32(4294967295)},
		{"uint64", int64(-1), uint64(math.MaxUint64)},
		{"uint", int64(-1), uint(math.MaxUint64)},
		{"float32", float64(0.5), float32(0.5)},
		{"float32", int64(3), float32(3)},
	} {
		w, ok := model.GoWidthNamed(tc.width)
		if !ok {
			t.Fatalf("GoWidthNamed(%q) = false", tc.width)
		}
		if got := w.Narrow(tc.value); got != tc.want {
			t.Errorf("%s.Narrow(%#v) = %#v, want %#v", tc.width, tc.value, got, tc.want)
		}
	}
}

// TestGoWidthZero covers the value a width holds before anything writes to it,
// the one an element of a slice of it reads back.
func TestGoWidthZero(t *testing.T) {
	t.Parallel()
	for width, want := range map[string]any{
		"int8":    int8(0),
		"uint16":  uint16(0),
		"uint64":  uint64(0),
		"float32": float32(0),
	} {
		w, ok := model.GoWidthNamed(width)
		if !ok {
			t.Fatalf("GoWidthNamed(%q) = false", width)
		}
		if got := w.Zero(); got != want {
			t.Errorf("%s.Zero() = %#v, want %#v", width, got, want)
		}
	}
}

// TestGoStep covers ++ and -- at a width. The step wraps where go's does: the
// largest int8 steps up to the smallest, and the smallest unsigned value steps
// down to the largest, with no widening to a php float at either end.
func TestGoStep(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value any
		delta int64
		want  any
	}{
		{int8(127), 1, int8(-128)},
		{int8(-128), -1, int8(127)},
		{uint8(255), 1, uint8(0)},
		{uint8(0), -1, uint8(255)},
		{uint64(math.MaxUint64), 1, uint64(0)},
		{float32(1.5), 1, float32(2.5)},
		{float32(1.5), -1, float32(0.5)},
	} {
		got, ok := model.GoStep(tc.value, tc.delta)
		if !ok {
			t.Errorf("GoStep(%#v, %d) = false", tc.value, tc.delta)
			continue
		}
		if got != tc.want {
			t.Errorf("GoStep(%#v, %d) = %#v, want %#v", tc.value, tc.delta, got, tc.want)
		}
	}
	if got, ok := model.GoStep(int64(1), 1); ok {
		t.Errorf("GoStep(int64(1), 1) = %#v, want php's own increment", got)
	}
}
