package runner

import (
	"fmt"
	"math"
	"reflect"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
)

// This file is the PHP operator tables: comparison, identity, arithmetic,
// bitwise and the unary operators. Both engines read them, the interpreter
// directly and the bytecode host through flatHost.Binary and Unary and the
// two cannot answer a different value for one expression. The coercions the
// operators read (toInt, toFloat, phpString) live in convert.go.

// phpLooseEqual is PHP's `==`, used by switch/case matching and the flatstack
// host. It reads the same table as the comparison operators and null equals 0,
// "" and false, and two numeric strings compare numerically ("1" == "01").
func phpLooseEqual(a, b any) bool {
	return phpCompare("==", a, b)
}

// phpCompare applies one comparison operator with PHP 8 semantics. The loose
// operators read phpval.Compare's ordering; `===` and `!==` are identity and
// never coerce.
func phpCompare(op string, a, b any) bool {
	switch op {
	case "===":
		return phpIdentical(a, b)
	case "!==":
		return !phpIdentical(a, b)
	}
	// A NaN operand answers false to every operator except `!=`, which PHP does
	// not derive from its ordering (NAN <=> 1 and 1 <=> NAN are both 1). The
	// one escape is a bool or null on the other side, where both operands go
	// through the bool domain and (bool)NAN is true.
	if (isNaNValue(a) || isNaNValue(b)) && !inBoolDomain(a) && !inBoolDomain(b) {
		return op == "!="
	}
	c := phpval.Compare(a, b)
	switch op {
	case "==":
		return c == 0
	case "!=":
		return c != 0
	case "<":
		return c < 0
	case "<=":
		return c <= 0
	case ">":
		return c > 0
	default:
		return c >= 0
	}
}

// isNaNValue reports a float64 NaN operand, the value fdiv(0, 0) returns.
func isNaNValue(v any) bool {
	f, ok := v.(float64)
	return ok && math.IsNaN(f)
}

// inBoolDomain reports an operand that drags a loose comparison into the bool
// domain: nil or bool, phpval.Compare's isBoolish.
func inBoolDomain(v any) bool {
	if v == nil {
		return true
	}
	_, ok := v.(bool)
	return ok
}

// phpIdentical is PHP's `===`: same type and same value, no coercion. The one
// widening is int against int64, which are a single PHP type (int) that
// arrives in two Go spellings: int64 from the parser and arithmetic, int from
// a Go binding. Arrays compare pairwise in order, objects by instance, and a
// host value a binding returned falls back to reflect equality on its own
// type.
func phpIdentical(a, b any) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case string:
		if y, ok := phpval.Bytes(b); ok {
			return x == y
		}
		y, ok := b.(string)
		return ok && x == y
	case []byte:
		// A []byte is the string it carries and `===` against one compares text
		// and never reports two different types. See phpval.Bytes.
		if y, ok := phpval.Bytes(b); ok {
			return string(x) == y
		}
		y, ok := b.(string)
		return ok && string(x) == y
	case int, int64:
		switch b.(type) {
		case int, int64:
			return toInt(a) == toInt(b)
		}
		return false
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case *model.Array:
		y, ok := b.(*model.Array)
		return ok && identicalArrays(x, y)
	case *model.Object:
		// PHP's `===` on objects is same instance, not equal properties.
		y, ok := b.(*model.Object)
		return ok && x == y
	}
	if b == nil {
		return false
	}
	// reflect.DeepEqual and not ==: a binding's []string is not comparable
	// and would panic under the operator.
	return reflect.TypeOf(a) == reflect.TypeOf(b) && reflect.DeepEqual(a, b)
}

// identicalArrays reports whether two arrays are identical the way PHP's `===`
// asks: the same key/value pairs, in the same order, with identical values.
func identicalArrays(x, y *model.Array) bool {
	if x == y {
		return true
	}
	if x.Len() != y.Len() {
		return false
	}
	xk, yk := x.Keys(), y.Keys()
	for i := range xk {
		if !phpIdentical(xk[i], yk[i]) {
			return false
		}
		xv, _ := x.Get(xk[i])
		yv, _ := y.Get(yk[i])
		if !phpIdentical(xv, yv) {
			return false
		}
	}
	return true
}

// phpArith applies + - * / % with the same int coercion used by += and -=.
func phpArith(op string, a, b any) any {
	// `+` on two arrays is PHP's union, not addition: every entry of the left
	// operand, plus the entries of the right whose keys it does not already
	// have. composer's ClassLoader prepends a loader with it.
	if op == "+" {
		if left, ok := a.(*model.Array); ok {
			if right, ok := b.(*model.Array); ok {
				return unionArrays(left, right)
			}
		}
	}
	// A numeric string operand is its number, keeping PHP's int/float
	// distinction by the string's own spelling: "5.5" + 1 is float 6.5 and
	// "1e2" * 2 is float 200. A string with trailing junk ("5x") stays a
	// string for the prefix conversion below.
	if s, ok := a.(string); ok {
		if n, ok := phpval.StringNumber(s); ok {
			a = n
		}
	}
	if s, ok := b.(string); ok {
		if n, ok := phpval.StringNumber(s); ok {
			b = n
		}
	}
	// `%` casts both operands to int, as PHP's modulo does.
	if op == "%" {
		y := toInt(b)
		if y == 0 {
			return int64(0)
		}
		return toInt(a) % y
	}
	if op == "**" {
		return phpPow(a, b)
	}
	// A float operand makes the whole expression float, as in PHP.
	if isFloat(a) || isFloat(b) {
		x, y := toFloat(a), toFloat(b)
		switch op {
		case "+":
			return x + y
		case "-":
			return x - y
		case "*":
			return x * y
		case "/":
			if y == 0 {
				return float64(0)
			}
			return x / y
		default:
			return float64(0)
		}
	}
	// Integer arithmetic that overflows becomes float in PHP and
	// PHP_INT_MAX + 1 is 9.2233720368548E+18, with no wrap to a negative. The
	// rules live in phpval so the flat VM's integer opcodes share them.
	x, y := toInt(a), toInt(b)
	switch op {
	case "+":
		return phpval.AddInt(x, y)
	case "-":
		return phpval.SubInt(x, y)
	case "*":
		return phpval.MulInt(x, y)
	case "/":
		return phpval.DivInt(x, y)
	default:
		return int64(0)
	}
}

// phpBitwise applies & | ^ << >>.
//
// phpBitwise casts both operands to int, except that & | ^ between two strings
// operate bytewise and return a string, as they do in PHP: "a" | "b" is "c".
// The operand length follows PHP: `&` and `^` stop at the shorter operand, `|`
// keeps the longer one and treats the missing bytes as zero.
//
// Shifts are int64 operations. A count of 64 or more needs no special case: Go
// defines an over-wide shift as 0 for `<<`, and 0 or -1 by sign for `>>`, which
// is what PHP produces. A negative count is the one input with no answer, and
// PHP raises ArithmeticError for it and picks neither.
func phpBitwise(op string, a, b any) (any, error) {
	switch op {
	case "&", "|", "^":
		if left, ok := a.(string); ok {
			if right, ok := b.(string); ok {
				return bitwiseString(op, left, right), nil
			}
		}
		x, y := toInt(a), toInt(b)
		switch op {
		case "&":
			return x & y, nil
		case "|":
			return x | y, nil
		default:
			return x ^ y, nil
		}
	case "<<", ">>":
		x, count := toInt(a), toInt(b)
		if count < 0 {
			return nil, &ArithmeticError{Message: "Bit shift by negative number"}
		}
		if op == "<<" {
			return x << uint64(count), nil
		}
		return x >> uint64(count), nil
	default:
		return nil, fmt.Errorf("unsupported bitwise operator %q", op)
	}
}

// bitwiseString applies & | ^ bytewise to two strings.
func bitwiseString(op string, a, b string) string {
	n := min(len(a), len(b))
	if op == "|" {
		n = max(len(a), len(b))
	}
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		var x, y byte
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		switch op {
		case "&":
			out[i] = x & y
		case "|":
			out[i] = x | y
		default:
			out[i] = x ^ y
		}
	}
	return string(out)
}

// phpNegate implements PHP's unary minus.
//
// phpNegate negates a float directly and never computes it as `0 - x`, which
// loses the sign of zero: PHP echoes -0.0 as -0. Everything else resolves
// through the arithmetic helper so that negating the smallest int64 overflows
// to a float the way every other arithmetic operator does, with no wrap back
// to itself with Go's int64 negation.
func phpNegate(v any) any {
	if f, ok := v.(float64); ok {
		return -f
	}
	return phpArith("-", int64(0), v)
}

// phpBitNot implements `~`. On a string it flips every byte; on anything else it
// casts to int, where ~n is -(n+1).
//
// PHP 8 raises a TypeError for a null, bool or array operand. phpscript casts
// those the way it casts every other bitwise operand and `~null` is -1 and no
// fatal error (docs/README.md).
func phpBitNot(v any) any {
	if s, ok := v.(string); ok {
		out := make([]byte, len(s))
		for i := 0; i < len(s); i++ {
			out[i] = ^s[i]
		}
		return string(out)
	}
	return ^toInt(v)
}

// phpPow implements `**`. Two int operands with a non-negative exponent stay
// int (2 ** 10 is int 1024) unless the result overflows; a float operand or a
// negative exponent makes the result float, as in PHP.
func phpPow(a, b any) any {
	if isFloat(a) || isFloat(b) {
		return math.Pow(toFloat(a), toFloat(b))
	}
	base, exp := toInt(a), toInt(b)
	if exp < 0 {
		return math.Pow(float64(base), float64(exp))
	}
	result, sq := int64(1), base
	for e := exp; e > 0; e >>= 1 {
		if e&1 == 1 {
			r, ok := phpval.OverflowMulInt(result, sq)
			if !ok {
				return math.Pow(float64(base), float64(exp))
			}
			result = r
		}
		if e > 1 {
			s, ok := phpval.OverflowMulInt(sq, sq)
			if !ok {
				return math.Pow(float64(base), float64(exp))
			}
			sq = s
		}
	}
	return result
}

// isFloat reports whether the value is a PHP float operand.
func isFloat(v any) bool {
	_, ok := v.(float64)
	return ok
}

// unionArrays implements PHP's array `+`. The left operand wins every key it
// has; the right contributes only what is missing. Insertion order follows the
// left array and then the surviving entries of the right, as PHP's
// own union preserves.
func unionArrays(left, right *model.Array) *model.Array {
	out := model.NewArraySize(left.Len() + right.Len())
	left.Range(func(key, val any) bool {
		out.Set(key, val)
		return true
	})
	right.Range(func(key, val any) bool {
		if _, exists := out.Get(key); !exists {
			out.Set(key, val)
		}
		return true
	})
	return out
}
