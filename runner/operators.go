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
// two cannot answer a different value for one expression. convert.go holds the
// coercions the operators read (toInt, toFloat, phpString).

// phpLooseEqual answers PHP's ==, used by switch/case and the flatstack host.
func phpLooseEqual(a, b any) bool {
	return phpCompare("==", a, b)
}

// phpCompare applies one PHP 8 comparison, where === and !== test identity alone.
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

// phpSpaceship answers -1, 0 or 1 by phpval.Compare, a NaN answering 1.
func phpSpaceship(a, b any) int64 {
	if (isNaNValue(a) || isNaNValue(b)) && !inBoolDomain(a) && !inBoolDomain(b) {
		return 1
	}
	switch c := phpval.Compare(a, b); {
	case c < 0:
		return -1
	case c > 0:
		return 1
	}
	return 0
}

// isNaNValue reports a float64 NaN operand, the value fdiv(0, 0) returns.
func isNaNValue(v any) bool {
	f, ok := v.(float64)
	return ok && math.IsNaN(f)
}

// inBoolDomain reports a nil or bool operand, which forces the bool domain.
func inBoolDomain(v any) bool {
	if v == nil {
		return true
	}
	_, ok := v.(bool)
	return ok
}

// phpIdentical answers PHP's === and folds int and int64 to a single type.
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
		// A []byte is the string it holds and `===` against one compares text
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

// identicalArrays reports the same key/value pairs in the same order, compared identically.
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
func phpArith(op string, a, b any) (any, error) {
	// `+` on two arrays is PHP's union, not addition: every entry of the left
	// operand, plus the entries of the right whose keys it does not already
	// have. composer's ClassLoader prepends a loader with it.
	if op == "+" {
		if left, ok := a.(*model.Array); ok {
			if right, ok := b.(*model.Array); ok {
				return unionArrays(left, right), nil
			}
		}
	}
	// A typed go value keeps its type and wraps at its width, which php has
	// no arithmetic for. See operators_gowidth.go.
	if result, typed, err := goArith(op, a, b); typed {
		return result, err
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
			return nil, &DivisionByZeroError{Message: "Modulo by zero"}
		}
		return toInt(a) % y, nil
	}
	if op == "**" {
		return phpPow(a, b), nil
	}
	// A float operand makes the whole expression float, as in PHP.
	if isFloat(a) || isFloat(b) {
		x, y := toFloat(a), toFloat(b)
		switch op {
		case "+":
			return x + y, nil
		case "-":
			return x - y, nil
		case "*":
			return x * y, nil
		case "/":
			if y == 0 {
				return nil, &DivisionByZeroError{Message: "Division by zero"}
			}
			return x / y, nil
		default:
			return float64(0), nil
		}
	}
	// Integer arithmetic that overflows becomes float in PHP and
	// PHP_INT_MAX + 1 is 9.2233720368548E+18, with no wrap to a negative. The
	// phpval holds the rules so the flat VM's integer opcodes share them.
	x, y := toInt(a), toInt(b)
	switch op {
	case "+":
		return phpval.AddInt(x, y), nil
	case "-":
		return phpval.SubInt(x, y), nil
	case "*":
		return phpval.MulInt(x, y), nil
	case "/":
		if y == 0 {
			return nil, &DivisionByZeroError{Message: "Division by zero"}
		}
		return phpval.DivInt(x, y), nil
	default:
		return int64(0), nil
	}
}

// phpBitwise applies & | ^ << >>, casting to int except & | ^ between two strings.
func phpBitwise(op string, a, b any) (any, error) {
	switch op {
	case "&", "|", "^":
		if left, ok := a.(string); ok {
			if right, ok := b.(string); ok {
				return bitwiseString(op, left, right), nil
			}
		}
		if result, typed, err := goBitwise(op, a, b); typed {
			return result, err
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
		if result, typed, err := goBitwise(op, a, b); typed {
			return result, err
		}
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

// phpNegate negates a float directly and everything else through phpArith.
func phpNegate(v any) any {
	if f, ok := v.(float64); ok {
		return -f
	}
	// A typed go value negates at its own width, which wraps where php's
	// widens: -int8(-128) is int8(-128).
	if negated, typed := goNegate(v); typed {
		return negated
	}
	result, _ := phpArith("-", int64(0), v)
	return result
}

// phpBitNot flips every byte of a string and casts anything else to int.
func phpBitNot(v any) any {
	if s, ok := v.(string); ok {
		out := make([]byte, len(s))
		for i := 0; i < len(s); i++ {
			out[i] = ^s[i]
		}
		return string(out)
	}
	// A typed go value inverts within its width: ~uint8(0) is uint8(255),
	// where php's int answers -1.
	if inverted, typed := goBitNot(v); typed {
		return inverted
	}
	return ^toInt(v)
}

// phpPow keeps ** an int unless an operand is float or exponent negative.
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

// unionArrays implements array +, the left operand winning every key it holds.
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
