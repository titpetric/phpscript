package runner

import (
	"math"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestPHPString covers the three renderings kept inline because concatenation
// and echo run through them: a string is itself, an int64 has one spelling, and
// a caught exception prints its error text. Everything that has ever disagreed
// about a rendering goes to phpval, so echo, implode and var_dump cannot
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
// collection is falsey whatever its type, so a binding returning a []string
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
// as the integer it carries, so `$t->month() + 1` counts months.
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
// string reads its numeric prefix, so (float)"2.5kg" is 2.5 and a string with
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

// TestPHPCompare covers the loose operators. null equals 0, "" and false; two
// numeric strings compare numerically, so "1" == "01". A NaN operand answers
// false to every operator except `!=`, which PHP does not derive from its
// ordering, and a bool or null on the other side pulls both operands through
// the bool domain where (bool)NAN is true.
func TestPHPCompare(t *testing.T) {
	t.Parallel()
	nan := math.NaN()
	for _, tc := range []struct {
		op   string
		a, b any
		want bool
	}{
		{"==", nil, int64(0), true},
		{"==", nil, "", true},
		{"==", nil, false, true},
		{"==", "1", "01", true},
		{"==", "1", "1x", false},
		{"!=", int64(1), int64(2), true},
		{"<", int64(1), int64(2), true},
		{"<=", int64(2), int64(2), true},
		{">", int64(3), int64(2), true},
		{">=", int64(2), int64(2), true},
		{"==", nan, int64(1), false},
		{"<", nan, int64(1), false},
		{">", nan, int64(1), false},
		{"!=", nan, int64(1), true},
		{"==", nan, true, true},
		{"===", int64(1), 1.0, false},
		{"!==", int64(1), 1.0, true},
	} {
		if got := phpCompare(tc.op, tc.a, tc.b); got != tc.want {
			t.Errorf("phpCompare(%q, %#v, %#v) = %v, want %v", tc.op, tc.a, tc.b, got, tc.want)
		}
	}
}

// TestPHPLooseEqual covers the entry switch/case matching and the flatstack
// host read. It resolves to the same table the comparison operators read, so a
// `switch` arm cannot match on a rule `==` disagrees with.
func TestPHPLooseEqual(t *testing.T) {
	t.Parallel()
	if !phpLooseEqual("1", int64(1)) {
		t.Error(`phpLooseEqual("1", 1) = false`)
	}
	if phpLooseEqual("1", int64(2)) {
		t.Error(`phpLooseEqual("1", 2) = true`)
	}
	if got, want := phpLooseEqual(nil, int64(0)), phpCompare("==", nil, int64(0)); got != want {
		t.Errorf("phpLooseEqual() = %v where phpCompare(\"==\") = %v", got, want)
	}
}

// TestPHPIdentical covers `===`: same type, same value, no coercion. int and
// int64 are one PHP type arriving in two Go spellings, a []byte is the string
// it carries, arrays compare pairwise in order, and objects compare by
// instance. A binding's []string falls back to reflect equality, because `==`
// on a slice would panic.
func TestPHPIdentical(t *testing.T) {
	t.Parallel()
	first := model.NewArray()
	first.Append(int64(1))
	second := model.NewArray()
	second.Append(int64(1))
	reordered := model.NewArray()
	reordered.Set("b", int64(2))
	reordered.Set("a", int64(1))
	ordered := model.NewArray()
	ordered.Set("a", int64(1))
	ordered.Set("b", int64(2))
	obj := model.NewObject(&model.Class{Name: "Store"})

	for _, tc := range []struct {
		name string
		a, b any
		want bool
	}{
		{"null against null", nil, nil, true},
		{"null against zero", nil, int64(0), false},
		{"int against int64", 1, int64(1), true},
		{"int against float", int64(1), 1.0, false},
		{"two equal floats", 2.5, 2.5, true},
		{"bytes against a string", []byte("ab"), "ab", true},
		{"a string against bytes", "ab", []byte("ab"), true},
		{"two equal arrays", first, second, true},
		{"the same array", first, first, true},
		{"arrays in another order", ordered, reordered, false},
		{"the same object", obj, obj, true},
		{"two objects of one class", obj, model.NewObject(obj.Class), false},
		{"two equal slices", []string{"a"}, []string{"a"}, true},
		{"two different slices", []string{"a"}, []string{"b"}, false},
		{"a slice against nil", []string{"a"}, nil, false},
		{"two bools", true, true, true},
	} {
		if got := phpIdentical(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: phpIdentical() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestHelperCast covers `(bool)`, `(int)`, `(float)`, `(string)` and `(array)`.
// An array casts to int 1 when it holds anything and 0 when it does not, which
// is the only context converting one: php refuses arithmetic on an array with a
// TypeError, so the rule lives in the cast.
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
// object with nothing in it, and an object passes through untouched, so
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

// TestPHPArith covers + - * / % **. `+` on two arrays is PHP's union, where the
// left operand wins every key it has; a numeric string operand keeps PHP's
// int/float distinction by its own spelling; integer arithmetic that overflows
// becomes float, so PHP_INT_MAX + 1 has no wrap to a negative.
func TestPHPArith(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		op   string
		a, b any
		want any
	}{
		{"integer addition", "+", int64(2), int64(3), int64(5)},
		{"a float operand", "+", int64(2), 0.5, 2.5},
		{"a float string", "+", "5.5", int64(1), 6.5},
		{"exponent syntax", "*", "1e2", int64(2), 200.0},
		{"subtraction", "-", int64(2), int64(3), int64(-1)},
		{"modulo", "%", int64(7), int64(3), int64(1)},
		{"modulo truncates", "%", 7.9, int64(3), int64(1)},
		{"integer power", "**", int64(2), int64(10), int64(1024)},
		{"a negative exponent", "**", int64(2), int64(-1), 0.5},
		{"overflow becomes float", "+", int64(math.MaxInt64), int64(1), float64(math.MaxInt64) + 1},
	} {
		if got := phpArith(tc.op, tc.a, tc.b); got != tc.want {
			t.Errorf("%s: phpArith(%q) = %#v, want %#v", tc.name, tc.op, got, tc.want)
		}
	}
}

// TestUnionArrays covers `+` on two arrays. Insertion order follows the left
// array and then the surviving entries of the right, which is the order PHP's
// own union preserves; composer's ClassLoader prepends a loader with it.
func TestUnionArrays(t *testing.T) {
	t.Parallel()
	left := model.NewArray()
	left.Append(int64(1))
	left.Append(int64(2))
	right := model.NewArray()
	right.Append(int64(9))
	right.Append(int64(9))
	right.Append(int64(9))

	out := phpArith("+", left, right).(*model.Array)
	if out.Len() != 3 {
		t.Fatalf("the union has %d entries, want 3", out.Len())
	}
	for key, want := range map[int64]any{0: int64(1), 1: int64(2), 2: int64(9)} {
		if value, _ := out.Get(key); value != want {
			t.Errorf("the union holds %#v at key %d, want %#v", value, key, want)
		}
	}
}

// TestPHPBitwise covers & | ^ << >>. Two strings operate bytewise and answer a
// string, where `&` and `^` stop at the shorter operand and `|` keeps the
// longer one with the missing bytes read as zero. A negative shift count has no
// answer, so PHP raises ArithmeticError for it.
func TestPHPBitwise(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		op   string
		a, b any
		want any
	}{
		{"&", int64(6), int64(3), int64(2)},
		{"|", int64(6), int64(3), int64(7)},
		{"^", int64(6), int64(3), int64(5)},
		{"|", "a", "b", "c"},
		{"&", "abc", "abd", "ab`"},
		{"|", "ab", "abcd", "abcd"},
		{"^", "ab", "abcd", "\x00\x00"},
		{"<<", int64(1), int64(3), int64(8)},
		{">>", int64(-8), int64(1), int64(-4)},
		{"<<", int64(1), int64(64), int64(0)},
		{">>", int64(-1), int64(64), int64(-1)},
	} {
		got, err := phpBitwise(tc.op, tc.a, tc.b)
		if err != nil {
			t.Errorf("phpBitwise(%q, %#v, %#v) failed: %v", tc.op, tc.a, tc.b, err)
			continue
		}
		if got != tc.want {
			t.Errorf("phpBitwise(%q, %#v, %#v) = %#v, want %#v", tc.op, tc.a, tc.b, got, tc.want)
		}
	}

	if _, err := phpBitwise("<<", int64(1), int64(-1)); err == nil {
		t.Error("a negative shift count reported no error")
	}
	if _, err := phpBitwise("??", int64(1), int64(1)); err == nil {
		t.Error("an unsupported operator reported no error")
	}
}

// TestPHPBitNot covers `~`. On a string it flips every byte; on anything else
// it casts to int, where ~n is -(n+1). PHP 8 raises a TypeError for a null,
// bool or array operand and phpscript casts those, so `~null` is -1. See
// docs/README.md.
func TestPHPBitNot(t *testing.T) {
	t.Parallel()
	if got, want := phpBitNot("A"), "\xbe"; got != want {
		t.Errorf("phpBitNot(%q) = %q, want %q", "A", got, want)
	}
	if got, want := phpBitNot(int64(5)), int64(-6); got != want {
		t.Errorf("phpBitNot(5) = %v, want %v", got, want)
	}
	if got, want := phpBitNot(nil), int64(-1); got != want {
		t.Errorf("phpBitNot(null) = %v, want %v", got, want)
	}
}

// TestPHPNegate covers unary minus. phpNegate negates a float directly and
// never computes it as `0 - x`, which loses the sign of zero: PHP echoes -0.0
// as -0. The smallest int64 overflows to a float the way every other
// arithmetic operator overflows, with no wrap back to itself.
func TestPHPNegate(t *testing.T) {
	t.Parallel()
	if got, want := phpNegate(int64(5)), int64(-5); got != want {
		t.Errorf("phpNegate(5) = %v, want %v", got, want)
	}

	zero, ok := phpNegate(0.0).(float64)
	if !ok {
		t.Fatalf("phpNegate(0.0) answered %T, want a float64", phpNegate(0.0))
	}
	if !math.Signbit(zero) {
		t.Error("phpNegate(0.0) lost the sign of zero")
	}

	if got, want := phpNegate(int64(math.MinInt64)), -float64(math.MinInt64); got != want {
		t.Errorf("phpNegate(MinInt64) = %#v, want %#v", got, want)
	}
}

// TestKeyString covers the name keyString reads off a model.Keyed. PHP
// normalises an index to an int or a string and a Keyed carries a string name,
// so an int index reads as its decimal spelling.
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
// string keys and nothing else, so a numeric string becomes the int it spells
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
