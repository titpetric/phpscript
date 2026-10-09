package phpval

import (
	"math"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// scriptArray builds a list-mode *model.Array of the given values, for the
// cases that have to be a script array rather than a Go slice.
func scriptArray(values ...any) *model.Array {
	out := model.NewArraySize(len(values))
	for _, v := range values {
		out.Append(v)
	}
	return out
}

// TestString pins each value against what php prints for `(string)$v`.
//
// The oracle used to be a second Go implementation rendering a float and an
// unknown type with fmt's %v, so the shortest round-tripping float
// form and Go's struct dump became the expected answers. A php name is a
// behaviour claim settled by php, so the want column is php's output for the
// same list, pasted.
func TestString(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "nil", value: nil, want: ""},
		{name: "empty string", value: "", want: ""},
		{name: "text", value: "text", want: "text"},
		{name: "true", value: true, want: "1"},
		{name: "false", value: false, want: ""},
		{name: "int64 zero", value: int64(0), want: "0"},
		{name: "int64 negative", value: int64(-1), want: "-1"},
		{name: "int64 4096", value: int64(4096), want: "4096"},
		{name: "int64 max", value: int64(math.MaxInt64), want: "9223372036854775807"},
		{name: "int64 min", value: int64(math.MinInt64), want: "-9223372036854775808"},
		{name: "int zero", value: 0, want: "0"},
		{name: "int negative", value: -1, want: "-1"},
		{name: "int 4096", value: 4096, want: "4096"},
		{name: "float zero", value: 0.0, want: "0"},
		{name: "float 1.5", value: 1.5, want: "1.5"},
		{name: "float negative", value: -0.25, want: "-0.25"},
		{name: "float 1e21", value: 1e21, want: "1.0E+21"},
		{name: "float 1e-7", value: 1e-7, want: "1.0E-7"},
		{name: "infinity", value: math.Inf(1), want: "INF"},
		{name: "not a number", value: math.NaN(), want: "NAN"},
		{name: "list of one", value: []string{"a"}, want: "Array"},
		{name: "script array", value: scriptArray("a"), want: "Array"},
		{name: "nested script array", value: scriptArray(scriptArray("a")), want: "Array"},
		{name: "map", value: map[string]any{"k": 1}, want: "Array"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := String(tt.value); got != tt.want {
				t.Errorf("String(%#v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestStringPrecisionMatchesEcho pins the reason the renderer moved here: the
// runner rendered a float at php's precision of 14 and this rendered the
// shortest form that round-trips, so one value printed two ways depending on
// whether it went through echo or through a binding.
func TestStringPrecisionMatchesEcho(t *testing.T) {
	tests := map[float64]string{
		0.1 * 0.2:   "0.02",
		1.0 / 3.0:   "0.33333333333333",
		1e20:        "1.0E+20",
		1e-7:        "1.0E-7",
		2.0:         "2",
		1234.5:      "1234.5",
		0.000123456: "0.000123456",
	}
	for in, want := range tests {
		if got := String(in); got != want {
			t.Errorf("String(%v) = %q, want %q", in, got, want)
		}
		if got := FloatString(in); got != want {
			t.Errorf("FloatString(%v) = %q, want %q", in, got, want)
		}
	}
}

// TestStringContextRefusesAnObject pins the one case a string context refuses
// and a key or a var_dump does not. There is no __toString here, so an object
// has no string form at all and php's Error is the whole of the case.
func TestStringContextRefusesAnObject(t *testing.T) {
	object := model.NewObject(&model.Class{Name: "Point"})
	_, err := StringContext(object)
	if err == nil {
		t.Fatal("StringContext(*model.Object) returned no error")
	}
	thrown, ok := err.(*ConversionError)
	if !ok {
		t.Fatalf("StringContext returned %T, want *ConversionError", err)
	}
	if got, want := thrown.Error(), "Object of class Point could not be converted to string"; got != want {
		t.Errorf("ConversionError.Error() = %q, want %q", got, want)
	}
	if class := thrown.ThrowableClass(); class != "Error" {
		t.Errorf("ThrowableClass() = %q, want %q", class, "Error")
	}
	// A class the object carries no declaration for still names something.
	bare := &ConversionError{}
	if got, want := bare.Error(), "Object of class stdClass could not be converted to string"; got != want {
		t.Errorf("ConversionError.Error() = %q, want %q", got, want)
	}
	// Everything with a string form goes through unchanged.
	for _, v := range []any{nil, "text", int64(7), 1.5, true, []string{"a"}} {
		got, err := StringContext(v)
		if err != nil {
			t.Fatalf("StringContext(%#v) returned %v", v, err)
		}
		if want := String(v); got != want {
			t.Errorf("StringContext(%#v) = %q, want %q", v, got, want)
		}
	}
}

// TestBytes pins the rule the value helpers ask about a []byte: it is the
// string it carries, so a binding returning one hands back text rather than a
// list of integers.
func TestBytes(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
		ok    bool
	}{
		{name: "text", value: []byte("alpha"), want: "alpha", ok: true},
		{name: "empty", value: []byte{}, want: "", ok: true},
		{name: "nil slice", value: []byte(nil), want: "", ok: true},
		{name: "a string is not one", value: "alpha", want: "", ok: false},
		{name: "another slice is not one", value: []string{"alpha"}, want: "", ok: false},
		{name: "nil", value: nil, want: "", ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := Bytes(test.value)
			if got != test.want || ok != test.ok {
				t.Errorf("Bytes(%#v) = %q, %v, want %q, %v", test.value, got, ok, test.want, test.ok)
			}
		})
	}

	// String and GoString agree with it, which is what echo and var_dump read.
	if got := String([]byte("alpha")); got != "alpha" {
		t.Errorf("String([]byte) = %q, want %q", got, "alpha")
	}
	if got, ok := GoString([]byte("alpha")); got != "alpha" || !ok {
		t.Errorf("GoString([]byte) = %q, %v, want %q, true", got, ok, "alpha")
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
