package phpval

import (
	"fmt"
	"math"
	"testing"
)

// replaced.
func TestString(t *testing.T) {
	values := []any{
		nil, "", "text", true, false,
		int64(0), int64(-1), int64(4096), int64(math.MaxInt64), int64(math.MinInt64),
		0, -1, 4096,
		0.0, 1.5, -0.25, 1e21, 1e-7, math.Inf(1), math.NaN(),
		[]string{"a"},
	}
	for _, v := range values {
		want := legacyToString(v)
		if got := String(v); got != want {
			t.Errorf("String(%#v) = %q, want %q", v, got, want)
		}
	}
}

func legacyToString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "1"
		}
		return ""
	case int64:
		return fmt.Sprintf("%d", x)
	case int:
		return fmt.Sprintf("%d", x)
	case float64:
		return fmt.Sprintf("%v", x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

// TestInt pins Int against PHP's own integer cast. Every expectation here was
// read from `php -r 'var_dump((int)$s);'` rather than from what the previous
// implementation happened to return.
func TestInt(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"0", 0},
		{"42", 42},
		{" 42", 42},
		{"\t42", 42},
		{"\n 42", 42},
		{"42abc", 42},
		{"abc", 0},
		{"-7", -7},
		{"+7", 7},
		{"0x1f", 0},
		{"3.9", 3},
		{"  -12  ", -12},
		{"007", 7},
		{"12 34", 12},
		{"- 5", 0},
		// An exponent is part of the numeric prefix, as it is to a PHP cast.
		{"1e3", 1000},
		{"1.5e2", 150},
		{"9223372036854775807", math.MaxInt64},
		{"-9223372036854775807", -math.MaxInt64},
		{"-9223372036854775808", math.MinInt64},
		// PHP saturates at the int64 bounds rather than wrapping or zeroing.
		{"99999999999999999999", math.MaxInt64},
		{"-99999999999999999999", math.MinInt64},
	}
	for _, tt := range tests {
		if got := Int(tt.in); got != tt.want {
			t.Errorf("Int(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
