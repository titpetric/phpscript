package runner

import (
	"math"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestPHPString covers the three renderings kept inline because concatenation
// and echo run through them: a string is itself, an int64 has one spelling, and
// a caught exception prints its error text. Everything that has ever disagreed
// about a rendering goes to phpval and echo, implode and var_dump cannot
// differ about a float's precision.
func TestPHPString(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"a string", "text", "text"},
		{"an int64", int64(-42), "-42"},
		{"an error", &TypeError{Name: "f", Position: 1, Want: "int", Got: "string"}, "f(): Argument #1 must be of type int, string given"},
		{"true", true, "1"},
		{"false", false, ""},
		{"null", nil, ""},
	} {
		if got := phpString(tc.value); got != tc.want {
			t.Errorf("%s: phpString() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestPHPTruthy covers what if/while/ternary reads. "0" is falsey where every
// other non-empty string is true, an empty array is falsey, and an empty Go
// collection is falsey whatever its type and a binding returning a []string
// behaves in `if ($rows)` the way one returning an array does.
func TestPHPTruthy(t *testing.T) {
	t.Parallel()
	filled := model.NewArray()
	filled.Append(int64(1))

	for _, tc := range []struct {
		name  string
		value any
		want  bool
	}{
		{"null", nil, false},
		{"false", false, false},
		{"true", true, true},
		{"an empty string", "", false},
		{"the string zero", "0", false},
		{"the string zero point zero", "0.0", true},
		{"a non-empty string", "no", true},
		{"zero", int64(0), false},
		{"a non-zero int", int64(-1), true},
		{"zero as a float", 0.0, false},
		{"an empty array", model.NewArray(), false},
		{"a filled array", filled, true},
		{"empty bytes", []byte(""), false},
		{"the bytes zero", []byte("0"), false},
		{"an empty slice", []string{}, false},
		{"a filled slice", []string{"a"}, true},
	} {
		if got := phpTruthy(tc.value); got != tc.want {
			t.Errorf("%s: phpTruthy() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestNumericPrefix covers PHP's string-to-number coercion, which takes the
// longest leading substring reading as a number and drops the rest: (int)"12abc"
// is 12 and (float)"2.5kg" is 2.5. The float flag separates "5.5" + 1, a float,
// from "5" + 1, an int.
func TestNumericPrefix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in      string
		prefix  string
		isFloat bool
	}{
		{"12abc", "12", false},
		{"2.5kg", "2.5", true},
		{"  -7.9z", "-7.9", true},
		{"+3", "+3", false},
		{"1e2", "1e2", true},
		{"1e", "1", false},
		{".5", ".5", true},
		{"abc", "", false},
		{"", "", false},
	} {
		prefix, isFloat := numericPrefix(tc.in)
		if prefix != tc.prefix || isFloat != tc.isFloat {
			t.Errorf("numericPrefix(%q) = (%q, %v), want (%q, %v)", tc.in, prefix, isFloat, tc.prefix, tc.isFloat)
		}
	}
}

// TestToInt covers the coercion an array index and `%` use. A float truncates
// toward zero, float syntax in a string truncates the same way, an out-of-range
// integer saturates the way C's strtol saturates, and a named Go scalar reads
// as the integer it carries and `$t->month() + 1` counts months.
func TestToInt(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value any
		want  int64
	}{
		{"an int", 7, 7},
		{"an int64", int64(-7), -7},
		{"a float", -7.9, -7},
		{"true", true, 1},
		{"false", false, 0},
		{"a numeric string", "12abc", 12},
		{"a float string", "  -7.9z", -7},
		{"a non-numeric string", "abc", 0},
		{"over int64", "99999999999999999999", math.MaxInt64},
		{"under int64", "-99999999999999999999", math.MinInt64},
		{"a named Go scalar", Size(4096), 4096},
		{"null", nil, 0},
	} {
		if got := toInt(tc.value); got != tc.want {
			t.Errorf("%s: toInt() = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestToFloat covers the float coercion `(float)` and a float operand use. A
// string reads its numeric prefix and (float)"2.5kg" is 2.5 and a string with
// no leading number is zero.
func TestToFloat(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		value any
		want  float64
	}{
		{"an int", 3, 3},
		{"an int64", int64(3), 3},
		{"a float", 2.5, 2.5},
		{"true", true, 1},
		{"a numeric string", "2.5kg", 2.5},
		{"a non-numeric string", "kg", 0},
		{"null", nil, 0},
	} {
		if got := toFloat(tc.value); got != tc.want {
			t.Errorf("%s: toFloat() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestHelperCast covers `(bool)`, `(int)`, `(float)`, `(string)` and `(array)`.
// An array casts to int 1 when it holds anything and 0 when it does not, which
// is the only context converting one: php refuses arithmetic on an array with a
// TypeError and the rule lives in the cast.
func TestHelperCast(t *testing.T) {
	t.Parallel()
	filled := model.NewArray()
	filled.Append(int64(9))

	for _, tc := range []struct {
		typ   string
		value any
		want  any
	}{
		{"bool", "0", false},
		{"boolean", "1", true},
		{"int", "12abc", int64(12)},
		{"integer", filled, int64(1)},
		{"int", model.NewArray(), int64(0)},
		{"float", "2.5kg", 2.5},
		{"double", int64(3), 3.0},
		{"string", int64(42), "42"},
		{"string", true, "1"},
		{"unknown", "as is", "as is"},
	} {
		if got := helperCast(tc.typ, tc.value); got != tc.want {
			t.Errorf("helperCast(%q, %#v) = %#v, want %#v", tc.typ, tc.value, got, tc.want)
		}
	}
}

// TestHelperCastArray covers `(array)`. Null produces an empty array, a scalar
// becomes a one-entry array, an object reads its properties out in the order
// the object reads them back, and a Go collection converts with its keys kept.
func TestHelperCastArray(t *testing.T) {
	t.Parallel()
	if got := helperCast("array", nil).(*model.Array); got.Len() != 0 {
		t.Errorf("(array) null has %d entries, want 0", got.Len())
	}

	wrapped := helperCast("array", "text").(*model.Array)
	if value, _ := wrapped.Get(int64(0)); value != "text" {
		t.Errorf("(array) on a scalar holds %#v at key 0, want the scalar", value)
	}

	obj := model.NewObject(&model.Class{Name: "Row", Fields: []model.Field{{Name: "id"}, {Name: "name"}}})
	obj.SetProp("name", "ada")
	obj.SetProp("id", int64(7))
	out := helperCast("array", obj).(*model.Array)
	if got, want := out.Keys(), []any{"id", "name"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("(array) on an object reads keys %v, want %v", got, want)
	}

	converted := helperCast("array", []string{"a", "b"}).(*model.Array)
	if converted.Len() != 2 {
		t.Errorf("(array) on a slice has %d entries, want 2", converted.Len())
	}
}

// TestHelperToObject covers `(object)`. An array becomes a stdClass whose
// properties are its entries, a scalar lands under `scalar`, null produces an
// object with nothing in it, and an object passes through untouched and
// `(object) $o === $o`.
func TestHelperToObject(t *testing.T) {
	t.Parallel()
	empty := helperToObject(nil).(*model.Object)
	if empty.Len() != 0 {
		t.Errorf("(object) null has %d properties, want 0", empty.Len())
	}

	arr := model.NewArray()
	arr.Set("name", "ada")
	converted := helperToObject(arr).(*model.Object)
	if value, ok := converted.Prop("name"); !ok || value != "ada" {
		t.Errorf("(object) on an array holds %#v under name", value)
	}

	scalar := helperToObject(int64(5)).(*model.Object)
	if value, ok := scalar.Prop("scalar"); !ok || value != int64(5) {
		t.Errorf("(object) on a scalar holds %#v under scalar", value)
	}

	obj := model.NewObject(&model.Class{Name: "Store"})
	if helperToObject(obj) != any(obj) {
		t.Error("(object) on an object did not answer the same instance")
	}
}

// TestKeyString covers the name keyString reads off a model.Keyed. PHP
// normalises an index to an int or a string and a Keyed carries a string name,
// and an int index reads as its decimal spelling.
func TestKeyString(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		key  any
		want string
	}{
		{int64(3), "3"},
		{"name", "name"},
		{"3", "3"},
		{true, "1"},
	} {
		if got := keyString(tc.key); got != tc.want {
			t.Errorf("keyString(%#v) = %q, want %q", tc.key, got, tc.want)
		}
	}
}

// TestNormalizeKey covers array key canonicalisation. PHP has int keys and
// string keys alone and a numeric string becomes the int it spells
// and a bool becomes 0 or 1. There is one definition of the rule, shared with
// array_key_exists() and array_flip().
func TestNormalizeKey(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		key  any
		want any
	}{
		{"3", int64(3)},
		{int64(3), int64(3)},
		{3, int64(3)},
		{"name", "name"},
		{true, int64(1)},
		{nil, ""},
	} {
		if got := normalizeKey(tc.key); got != tc.want {
			t.Errorf("normalizeKey(%#v) = %#v, want %#v", tc.key, got, tc.want)
		}
	}
}
