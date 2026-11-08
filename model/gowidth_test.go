package model_test

import (
	"math"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestGoWidthNamed covers the width table. A sized name answers its bits and
// its sign, go's two aliases answer the width they alias, and a name php
// already has - int, int64, float64 - answers nothing, because `int64(5)`
// is php's 5 and holds no width.
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

// TestGoWidth_Valid covers a width a host assembled by hand: it names a go
// numeric type and declares the size go declares for that name.
//
// The zero value fails. A caller that skipped GoWidthNamed holds one, and an
// unchecked zero width narrows every value to int64.
func TestGoWidth_Valid(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"int8", "uint64", "float32", "rune", "byte"} {
		w, _ := model.GoWidthNamed(name)
		if err := w.Valid(); err != nil {
			t.Errorf("GoWidthNamed(%q).Valid() = %v, want nil", name, err)
		}
	}
	for _, tc := range []struct {
		name  string
		width model.GoWidth
		want  string
	}{
		{"the zero value", model.GoWidth{}, "names no go numeric type"},
		{"a name go has none of", model.GoWidth{Name: "int128", Bits: 128, Signed: true}, "names no go numeric type"},
		{"php's own int", model.GoWidth{Name: "int64", Bits: 64, Signed: true}, "names no go numeric type"},
		{"the wrong width for the name", model.GoWidth{Name: "int8", Bits: 16, Signed: true}, "declares 16 bits"},
		{"the wrong signedness", model.GoWidth{Name: "uint8", Bits: 8, Signed: true}, "signed true"},
		{"an alias under its own name", model.GoWidth{Name: "rune", Bits: 32, Signed: true}, "declares 32 bits"},
	} {
		err := tc.width.Valid()
		if err == nil {
			t.Errorf("%s: Valid() = nil, want an error naming %q", tc.name, tc.want)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: Valid() = %v, want it to name %q", tc.name, err, tc.want)
		}
	}
}

// TestGoWidthOf covers the width a value holds. php's own types hold none,
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
