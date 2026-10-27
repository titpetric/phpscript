package runner

import (
	"math"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestPHPCompare covers the loose operators. null equals 0, "" and false; two
// numeric strings compare numerically and "1" == "01". A NaN operand answers
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
// host read. It resolves to the same table the comparison operators read and a
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

// TestPHPArith covers + - * / % **. `+` on two arrays is PHP's union, where the
// left operand wins every key it has; a numeric string operand keeps PHP's
// int/float distinction by its own spelling; integer arithmetic that overflows
// becomes float and PHP_INT_MAX + 1 has no wrap to a negative.
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
// answer and PHP raises ArithmeticError for it.
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
// bool or array operand and phpscript casts those and `~null` is -1. See
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
