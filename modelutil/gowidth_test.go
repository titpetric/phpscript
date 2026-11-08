package modelutil_test

import (
	"math"
	"testing"

	"github.com/titpetric/phpscript/modelutil"
)

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
		got, ok := modelutil.GoWiden(value)
		if !ok {
			t.Errorf("GoWiden(%#v) = false", value)
			continue
		}
		if got != want {
			t.Errorf("GoWiden(%#v) = %#v, want %#v", value, got, want)
		}
	}
	if got, ok := modelutil.GoWiden(int64(1)); ok {
		t.Errorf("GoWiden(int64(1)) = %#v, want no widening", got)
	}
}

// TestGoInt covers the bits an operation computes over. An unsigned value past
// PHP_INT_MAX keeps its bits here, where GoWiden reaches the float php holds
// it in, and narrowing it back reads the low byte it still has.
func TestGoInt(t *testing.T) {
	t.Parallel()
	bits, ok := modelutil.GoInt(uint64(math.MaxUint64))
	if !ok {
		t.Fatal("GoInt(uint64 max) = false")
	}
	if bits != -1 {
		t.Errorf("GoInt(uint64 max) = %d, want -1", bits)
	}
	unsigned, ok := modelutil.GoUint(uint64(math.MaxUint64))
	if !ok {
		t.Fatal("GoUint(uint64 max) = false")
	}
	if unsigned != math.MaxUint64 {
		t.Errorf("GoUint(uint64 max) = %d, want %d", unsigned, uint64(math.MaxUint64))
	}
	if _, ok := modelutil.GoInt(float32(1)); ok {
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
		got, ok := modelutil.GoClipInt(value)
		if !ok {
			t.Errorf("GoClipInt(%#v) = false", value)
			continue
		}
		if got != want {
			t.Errorf("GoClipInt(%#v) = %d, want %d", value, got, want)
		}
	}
	for _, value := range []any{float32(1), int64(1), "1", nil} {
		if got, ok := modelutil.GoClipInt(value); ok {
			t.Errorf("GoClipInt(%#v) = %d, want php's own reading", value, got)
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
		got, ok := modelutil.GoStep(tc.value, tc.delta)
		if !ok {
			t.Errorf("GoStep(%#v, %d) = false", tc.value, tc.delta)
			continue
		}
		if got != tc.want {
			t.Errorf("GoStep(%#v, %d) = %#v, want %#v", tc.value, tc.delta, got, tc.want)
		}
	}
	if got, ok := modelutil.GoStep(int64(1), 1); ok {
		t.Errorf("GoStep(int64(1), 1) = %#v, want php's own increment", got)
	}
}
