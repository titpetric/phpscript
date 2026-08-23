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

// TestInt pins leadingInt against the fmt.Sscanf("%d") behaviour it
// replaced, including the cases where a scan failed and left zero behind.
func TestInt(t *testing.T) {
	inputs := []string{
		"", "0", "42", " 42", "\t42", "42abc", "abc", "-7", "+7",
		"0x1f", "3.9", "  -12  ", "007", "12 34", "- 5", "1e3",
		"9223372036854775807", "-9223372036854775807",
		"99999999999999999999", "-99999999999999999999",
	}
	for _, in := range inputs {
		var want int64
		fmt.Sscanf(in, "%d", &want)
		if got := Int(in); got != want {
			t.Errorf("Int(%q) = %d, want %d", in, got, want)
		}
	}
	if got := Int("-9223372036854775808"); got != 0 && got != math.MinInt64 {
		t.Errorf("Int(MinInt64) = %d, want 0 or MinInt64", got)
	}
	// One deliberate divergence: Sscanf treats a newline as a terminator, so
	// it read "\n 42" as nothing. PHP's integer cast skips all leading
	// whitespace, which is what leadingInt does.
	if got := Int("\n 42"); got != 42 {
		t.Errorf("Int(%q) = %d, want 42", "\n 42", got)
	}
}
