package phpval

import (
	"errors"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestStrings covers the string context implode and str_replace read a
// collection in. Each element converts the way `(string)$v` does and php 8.5
// prints "1,1,,2.5" for implode(",", ["1", true, null, 2.5]).
func TestStrings(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		value any
		want  []string
	}{
		{name: "nil", value: nil},
		{name: "empty slice", value: []any{}},
		{name: "empty script array", value: scriptArray()},
		{name: "strings pass through", value: []string{"a", "b"}, want: []string{"a", "b"}},
		{name: "empty string slice", value: []string{}, want: []string{}},
		{name: "mixed values", value: scriptArray("1", true, nil, 2.5), want: []string{"1", "1", "", "2.5"}},
		{name: "ints", value: []any{int64(1), int64(2)}, want: []string{"1", "2"}},
		{name: "false is the empty string", value: []any{false}, want: []string{""}},
		{name: "nested array", value: scriptArray(scriptArray("a")), want: []string{"Array"}},
		{name: "map keeps values only", value: map[string]any{"k": "v"}, want: []string{"v"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Strings(tt.value)
			if err != nil {
				t.Fatalf("Strings(%#v) = %v", tt.value, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("Strings(%#v) = %#v, want %#v", tt.value, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("Strings(%#v)[%d] = %q, want %q", tt.value, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestStringsRefusesObject covers the one element with no string form. php 8.5
// raises Error with the wording want pins below, and Strings reports the same
// wording and nothing invents a spelling for an object inside implode.
func TestStringsRefusesObject(t *testing.T) {
	t.Parallel()
	object := model.NewObject(&model.Class{Name: "C"})

	got, err := Strings(scriptArray("a", object, "b"))
	if got != nil {
		t.Errorf("Strings returned %#v alongside the error, want nil", got)
	}

	var conversion *ConversionError
	if !errors.As(err, &conversion) {
		t.Fatalf("Strings error = %v, want a ConversionError", err)
	}
	if conversion.Class != "C" {
		t.Errorf("the error names class %q, want C", conversion.Class)
	}
	want := "Object of class C could not be converted to string"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// TestValues covers the order a collection reads back in and the one case that
// copies no value: a []any is the storage the caller already holds and Values
// passes it through. Callers that write to the result copy first.
func TestValues(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		value any
		want  []any
	}{
		{name: "nil", value: nil},
		{name: "empty script array", value: scriptArray()},
		{name: "script array keeps order", value: scriptArray("a", int64(2), nil), want: []any{"a", int64(2), nil}},
		{name: "string slice", value: []string{"a", "b"}, want: []any{"a", "b"}},
		{name: "single entry map", value: map[string]any{"k": "v"}, want: []any{"v"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := Values(tt.value)
			if len(got) != len(tt.want) {
				t.Fatalf("Values(%#v) = %#v, want %#v", tt.value, got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("Values(%#v)[%d] = %#v, want %#v", tt.value, i, got[i], tt.want[i])
				}
			}
		})
	}

	// Values returns a []any as the same backing array and a caller reading it
	// sees the collection and no copy of it.
	source := []any{int64(1), int64(2)}
	returned := Values(source)
	source[0] = int64(9)
	if returned[0] != int64(9) {
		t.Errorf("Values copied a []any: returned[0] = %#v after the source changed", returned[0])
	}
}
