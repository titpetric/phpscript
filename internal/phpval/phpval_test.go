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

func TestFloat(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want float64
	}{
		{"leading digits", "12abc", 12},
		{"an exponent is part of the prefix", "1e3", 1000},
		{"surrounding space", "  7 ", 7},
		{"true", true, 1},
		{"false", false, 0},
		{"nil", nil, 0},
		{"leading zeroes", "007", 7},
		{"zero", "0", 0},
		{"negative fraction", "-3.5", -3.5},
		{"int", 5, 5},
		{"int64", int64(5), 5},
		{"whole float", 2.0, 2},
		{"empty", "", 0},
		{"no prefix", "abc", 0},
		{"bare fraction", ".5", 0.5},
		{"signed bare fraction", "+.5", 0.5},
		{"trailing point", "5.", 5},
		{"lone point", ".", 0},
		{"hex is a prefix of one digit", "0x1f", 0},
		{"fraction then letters", "1.5x", 1.5},
		{"plus sign", "+7", 7},
		{"space inside", "12 34", 12},
		{"detached sign", "- 5", 0},
		{"array", []string{"a"}, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Float(test.in); got != test.want {
				t.Errorf("Float(%#v) = %v, want %v", test.in, got, test.want)
			}
		})
	}
}

// TestFloatAgreesWithInt pins the invariant the leadingFloat/leadingInt pair
// exists for: whatever prefix one of them reads, the other reads the same
// number, so Int and Float can never disagree about a string.
func TestFloatAgreesWithInt(t *testing.T) {
	inputs := []string{
		"", "0", "42", " 42", "\t42", "42abc", "abc", "-7", "+7",
		"0x1f", "3.9", "  -12  ", "007", "12 34", "- 5", "1e3",
		".5", "-.5", "5.", ".", "1.5x", "\n 42",
	}
	for _, in := range inputs {
		if got, want := int64(Float(in)), Int(in); got != want {
			t.Errorf("int64(Float(%q)) = %d, Int(%q) = %d", in, got, in, want)
		}
	}
}

func TestNumber(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want any
	}{
		{"leading digits", "12abc", int64(12)},
		{"an exponent is part of the prefix", "1e3", float64(1000)},
		{"surrounding space", "  7 ", int64(7)},
		{"true", true, int64(1)},
		{"false", false, int64(0)},
		{"nil", nil, int64(0)},
		{"leading zeroes", "007", int64(7)},
		{"zero", "0", int64(0)},
		{"negative fraction", "-3.5", -3.5},
		{"int", 5, int64(5)},
		{"int64", int64(5), int64(5)},
		{"whole float stays a float", 2.0, 2.0},
		{"empty", "", int64(0)},
		{"no prefix", "abc", int64(0)},
		{"bare fraction", ".5", 0.5},
		{"trailing point", "5.", 5.0},
		{"integer string", "42", int64(42)},
		{"array", []string{"a"}, int64(0)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Number(test.in)
			if got != test.want {
				t.Errorf("Number(%#v) = %#v, want %#v", test.in, got, test.want)
			}
			// The type is the whole point: int64(1) and float64(1) are not
			// interchangeable to abs() or array_sum().
			if _, ok := got.(int64); !ok {
				if _, ok := got.(float64); !ok {
					t.Errorf("Number(%#v) = %T, want int64 or float64", test.in, got)
				}
			}
		})
	}
}

func TestNumberMatchesIntAndFloat(t *testing.T) {
	inputs := []any{
		"12abc", "1e3", "  7 ", true, false, nil, "007", "0", "-3.5",
		5, int64(5), 2.0, "", "abc", ".5", "5.",
	}
	for _, in := range inputs {
		switch n := Number(in).(type) {
		case int64:
			if n != Int(in) {
				t.Errorf("Number(%#v) = int64(%d), Int = %d", in, n, Int(in))
			}
		case float64:
			if n != Float(in) && !(math.IsNaN(n) && math.IsNaN(Float(in))) {
				t.Errorf("Number(%#v) = float64(%v), Float = %v", in, n, Float(in))
			}
		}
	}
}

// TestKey pins PHP's array key rules, read off php 8's output for
// `$a[<in>] = 1; foreach ($a as $k => $v)`.
func TestKey(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want any
	}{
		{"decimal string", "1", int64(1)},
		{"zero", "0", int64(0)},
		{"negative", "-2", int64(-2)},
		{"int widens", 1, int64(1)},
		{"int64 unchanged", int64(1), int64(1)},
		{"word", "abc", "abc"},
		{"empty string", "", ""},

		// Not canonical spellings, so not integer keys.
		{"leading zero", "01", "01"},
		{"leading zeros", "007", "007"},
		{"zero padded zero", "00", "00"},
		{"signed leading zero", "-01", "-01"},
		{"signed zero prints as 0", "-0", "-0"},
		{"explicit plus", "+1", "+1"},
		{"space is not a decimal", " 1", " 1"},
		{"fraction stays a string", "1.5", "1.5"},
		{"exponent is not a decimal", "1e3", "1e3"},
		{"past int64", "9223372036854775808", "9223372036854775808"},

		// The other three scalar types have integer or string keys too.
		{"true is 1", true, int64(1)},
		{"false is 0", false, int64(0)},
		{"nil is the empty string", nil, ""},
		{"float truncates toward zero", 1.5, int64(1)},
		{"negative float truncates toward zero", -1.7, int64(-1)},
		{"float below one is zero", 0.4, int64(0)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Key(test.in); got != test.want {
				t.Errorf("Key(%#v) = %#v, want %#v", test.in, got, test.want)
			}
		})
	}
}
