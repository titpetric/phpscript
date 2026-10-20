package http

import (
	"testing"
	"time"

	"github.com/titpetric/phpscript/model"
)

// TestToString covers how an option reaches a header value. PHP is dynamically
// typed and an option arrives as whatever the script wrote, so toString renders
// a number written where the header takes a string the way PHP renders it in a
// string context, and null is the empty string.
func TestToString(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		value any
		want  string
	}{
		{name: "nil", value: nil, want: ""},
		{name: "string", value: "Bearer token", want: "Bearer token"},
		{name: "empty string", value: "", want: ""},
		{name: "int64", value: int64(42), want: "42"},
		{name: "float", value: 1.5, want: "1.5"},
		{name: "true", value: true, want: "true"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := toString(test.value); got != test.want {
				t.Errorf("toString(%#v) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

// TestToBool covers PHP truthiness for the values a script passes an option.
// The string "false" is true in PHP's own cast and false here, because a script
// writing "false" in an option array meant the option off, and reading it
// PHP's way would turn every such option on.
func TestToBool(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		value any
		want  bool
	}{
		{name: "nil", value: nil},
		{name: "false", value: false},
		{name: "true", value: true, want: true},
		{name: "empty string", value: ""},
		{name: `"0"`, value: "0"},
		{name: `"false"`, value: "false"},
		{name: `"FALSE"`, value: "FALSE"},
		{name: `" off "`, value: " off "},
		{name: `"no"`, value: "no"},
		{name: `"1"`, value: "1", want: true},
		{name: `"yes"`, value: "yes", want: true},
		{name: "zero", value: int64(0)},
		{name: "one", value: int64(1), want: true},
		{name: "negative", value: int64(-1), want: true},
		{name: "zero float", value: 0.0},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := toBool(test.value); got != test.want {
				t.Errorf("toBool(%#v) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}

// TestToInt covers the numeric shapes an option arrives as. A script writes a
// literal and the parser emits an int64, a computed value arrives as a
// float64, and a value read out of a configuration file arrives as a string, so
// all three read as the same number.
func TestToInt(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		value any
		want  int64
	}{
		{name: "int64", value: int64(30), want: 30},
		{name: "int", value: 30, want: 30},
		{name: "float", value: 30.9, want: 30},
		{name: "numeric string", value: "30", want: 30},
		{name: "leading number", value: "30s", want: 30},
		{name: "negative", value: int64(-5), want: -5},
		{name: "true", value: true, want: 1},
		{name: "false", value: false},
		{name: "nil", value: nil},
		{name: "not a number", value: "none"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := toInt(test.value); got != test.want {
				t.Errorf("toInt(%#v) = %d, want %d", test.value, got, test.want)
			}
		})
	}
}

// TestToDuration covers how a script spells a timeout. A bare number is
// seconds, the spelling PHP's own timeouts use, and toDuration parses a string
// carrying a unit the way Go spells one, so a sub-second timeout needs no
// fraction.
func TestToDuration(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		value any
		want  time.Duration
	}{
		{name: "seconds as int64", value: int64(30), want: 30 * time.Second},
		{name: "seconds as float", value: 1.5, want: 1500 * time.Millisecond},
		{name: "numeric string", value: "30", want: 30 * time.Second},
		{name: "go duration", value: "500ms", want: 500 * time.Millisecond},
		{name: "compound duration", value: "1m30s", want: 90 * time.Second},
		{name: "padded", value: " 2s ", want: 2 * time.Second},
		{name: "nil", value: nil},
		{name: "unparseable", value: "soon"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := toDuration(test.value); got != test.want {
				t.Errorf("toDuration(%#v) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

// TestToHeaders covers the headers option, which a script writes as an
// associative array of name to value. toHeaders walks every PHP array shape
// the runtime returns through model.RangeValues, so a native map and a
// *model.Array read the same, and NewClient reports a value that is no array
// at all.
func TestToHeaders(t *testing.T) {
	t.Parallel()
	array := model.NewArray()
	array.Set("Authorization", "Bearer token")
	array.Set("X-Count", int64(2))

	for _, test := range []struct {
		name  string
		value any
		want  map[string]string
	}{
		{name: "nil", value: nil},
		{
			name:  "native map",
			value: map[string]any{"Authorization": "Bearer token"},
			want:  map[string]string{"Authorization": "Bearer token"},
		},
		{
			name:  "model array",
			value: array,
			want:  map[string]string{"Authorization": "Bearer token", "X-Count": "2"},
		},
		{
			name:  "empty array",
			value: model.NewArray(),
			want:  map[string]string{},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := toHeaders(test.value)
			if err != nil {
				t.Fatalf("toHeaders: %v", err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("toHeaders = %v, want %v", got, test.want)
			}
			for name, want := range test.want {
				if got[name] != want {
					t.Errorf("toHeaders[%q] = %q, want %q", name, got[name], want)
				}
			}
		})
	}

	// A scalar where the option takes an array is a typo the script sees reported.
	for _, value := range []any{"Authorization: Bearer token", int64(1), true} {
		if _, err := toHeaders(value); err == nil {
			t.Errorf("toHeaders(%#v) accepted a value that is no array", value)
		}
	}
}
