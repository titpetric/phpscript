package core

import (
	"testing"
)

// TestPhpSprintfCoercesArguments pins the part of sprintf a fixture cannot
// reach from the other side: that an argument is converted before it is
// rendered, and never passed to a Go formatter that reports its type back.
func TestPhpSprintfCoercesArguments(t *testing.T) {
	tests := []struct {
		name   string
		format string
		args   []any
		want   string
	}{
		{name: "int through %s", format: "[%s]", args: []any{int64(42)}, want: "[42]"},
		{name: "float through %s", format: "[%s]", args: []any{1.5}, want: "[1.5]"},
		{name: "bool through %s", format: "[%s]", args: []any{true}, want: "[1]"},
		{name: "null through %s", format: "[%s]", args: []any{nil}, want: "[]"},
		{name: "numeric string through %d", format: "[%d]", args: []any{"42abc"}, want: "[42]"},
		{name: "float through %d", format: "[%d]", args: []any{3.9}, want: "[3]"},
		{name: "int through %.2f", format: "[%.2f]", args: []any{int64(3)}, want: "[3.00]"},
		{name: "negative through %u", format: "%u", args: []any{int64(-1)}, want: "18446744073709551615"},
		{name: "negative through %x", format: "%x", args: []any{int64(-1)}, want: "ffffffffffffffff"},
		{name: "argument number", format: `%2$s %1$s`, args: []any{"a", "b"}, want: "b a"},
		{name: "named padding", format: "%'x10s", args: []any{"abc"}, want: "xxxxxxxabc"},
		{name: "zero padding splits the sign", format: "%05d", args: []any{int64(-42)}, want: "-0042"},
		{name: "literal percent", format: "100%%", args: nil, want: "100%"},
		{name: "precision counts bytes", format: "%.2s", args: []any{"\xc3\xa9x"}, want: "\xc3\xa9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := phpSprintf(tt.format, tt.args...)
			if err != nil {
				t.Fatalf("phpSprintf(%q) returned %v", tt.format, err)
			}
			if got != tt.want {
				t.Errorf("phpSprintf(%q) = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

// TestPhpSprintfErrors pins the class each malformed format raises, which is
// what decides the catch clause a script writes around it.
func TestPhpSprintfErrors(t *testing.T) {
	tests := []struct {
		name   string
		format string
		args   []any
		class  string
	}{
		{name: "unknown specifier", format: "%v", args: []any{int64(1)}, class: "ValueError"},
		{name: "missing argument", format: "%s %s", args: []any{"a"}, class: "ArgumentCountError"},
		{name: "argument number past the end", format: `%3$s`, args: []any{"a"}, class: "ArgumentCountError"},
		{name: "argument number zero", format: `%0$s`, args: []any{"a"}, class: "ValueError"},
		{name: "specifier missing at the end", format: "100%", args: []any{"a"}, class: "ValueError"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := phpSprintf(tt.format, tt.args...)
			if err == nil {
				t.Fatalf("phpSprintf(%q) returned no error", tt.format)
			}
			thrown, ok := err.(*formatError)
			if !ok {
				t.Fatalf("phpSprintf(%q) returned %T, want *formatError", tt.format, err)
			}
			if got := thrown.ThrowableClass(); got != tt.class {
				t.Errorf("ThrowableClass() = %q, want %q", got, tt.class)
			}
		})
	}
}

// TestTrimExponent pins the exponent form PHP writes, which is the digits it
// needs, and not the two Go and C pad it to.
func TestTrimExponent(t *testing.T) {
	tests := map[string]string{
		"1.234500e+03":  "1.234500e+3",
		"1.200000E-04":  "1.200000E-4",
		"0.000000e+00":  "0.000000e+0",
		"1.000000e+100": "1.000000e+100",
		"1.000000e+10":  "1.000000e+10",
		"1.500000":      "1.500000",
		"0":             "0",
	}
	for in, want := range tests {
		if got := trimExponent(in); got != want {
			t.Errorf("trimExponent(%q) = %q, want %q", in, got, want)
		}
	}
}
