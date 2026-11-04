package runner

import (
	"fmt"
	"math"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
)

// A typed go value keeps its type through an operation and wraps at its width,
// which is go's arithmetic and not php's: `int8(100) + int8(100)` is
// `int8(-56)` where php's integers widen to a float. Three rules decide the
// width of a result:
//
//   - two values of one width answer that width;
//   - a typed value and one of php's own numbers answer the typed width, the
//     way go reads an untyped constant, and the number converts to it;
//   - two different widths answer a TypeError, as go refuses `int8 + int16`
//     outright. An explicit conversion is what a script writes instead, and
//     php has no promotion rule to borrow.
//
// Two of php's own operands never reach here: goArith reports false and the
// caller keeps php's own arithmetic, overflow to float included.

// goArith applies an arithmetic operator at a typed value's width, and reports
// whether the operands carried one.
func goArith(op string, a, b any) (any, bool, error) {
	w, typed, err := goOperandWidth(op, a, b)
	if !typed || err != nil {
		return nil, typed, err
	}
	switch {
	case w.Float:
		return goFloatArith(op, w, a, b)
	case w.Signed:
		return goSignedArith(op, w, a, b)
	default:
		return goUnsignedArith(op, w, a, b)
	}
}

// goBitwise applies a bitwise operator at a typed value's width. A shift past
// the width leaves zero, as go's does, because the result narrows to it.
func goBitwise(op string, a, b any) (any, bool, error) {
	w, typed, err := goOperandWidth(op, a, b)
	if !typed || err != nil {
		return nil, typed, err
	}
	if w.Float {
		return nil, true, &OperandError{Message: fmt.Sprintf("Unsupported operand types: %s %s %s", w.Name, op, w.Name)}
	}
	x, y := goOperandInt(a), goOperandInt(b)
	switch op {
	case "&":
		return w.Narrow(x & y), true, nil
	case "|":
		return w.Narrow(x | y), true, nil
	case "^":
		return w.Narrow(x ^ y), true, nil
	case "<<":
		if y < 0 {
			return nil, true, &ArithmeticError{Message: "Bit shift by negative number"}
		}
		return w.Narrow(shiftLeft(x, y)), true, nil
	case ">>":
		if y < 0 {
			return nil, true, &ArithmeticError{Message: "Bit shift by negative number"}
		}
		return w.Narrow(shiftRight(w, x, y)), true, nil
	}
	return nil, false, nil
}

// goOperandWidth reads the width an operation answers in, and reports whether
// either operand carried one at all.
func goOperandWidth(op string, a, b any) (model.GoWidth, bool, error) {
	left, leftTyped := model.GoWidthOf(a)
	right, rightTyped := model.GoWidthOf(b)
	switch {
	case !leftTyped && !rightTyped:
		return model.GoWidth{}, false, nil
	case leftTyped && rightTyped && left.Name != right.Name:
		return model.GoWidth{}, true, &OperandError{
			Message: fmt.Sprintf("Unsupported operand types: %s %s %s", left.Name, op, right.Name),
		}
	case leftTyped:
		return left, true, nil
	default:
		return right, true, nil
	}
}

// goOperandInt reads an operand as the bits a width computes over: a typed
// value answers its own, and one of php's numbers converts the way go reads an
// untyped constant.
func goOperandInt(v any) int64 {
	if bits, ok := model.GoInt(v); ok {
		return bits
	}
	return phpval.Int(v)
}

// goOperandFloat is goOperandInt for a float width.
func goOperandFloat(v any) float64 {
	if f, ok := v.(float32); ok {
		return float64(f)
	}
	return phpval.Float(v)
}

// goSignedArith computes at a signed width. The carrier is an int64 and the
// result narrows to the width, which is the wrap go applies: int8(100) *
// int8(100) is 10000 narrowed, int8(16).
func goSignedArith(op string, w model.GoWidth, a, b any) (any, bool, error) {
	x, y := goOperandInt(a), goOperandInt(b)
	switch op {
	case "+":
		return w.Narrow(x + y), true, nil
	case "-":
		return w.Narrow(x - y), true, nil
	case "*":
		return w.Narrow(x * y), true, nil
	case "/":
		if y == 0 {
			return w.Zero(), true, nil
		}
		// go's division truncates toward zero and keeps the type, where php's
		// `/` answers a float for an inexact one: int8(7) / int8(2) is int8(3).
		return w.Narrow(x / y), true, nil
	case "%":
		if y == 0 {
			return w.Zero(), true, nil
		}
		return w.Narrow(x % y), true, nil
	case "**":
		return w.Narrow(phpval.ToInt64(math.Pow(float64(x), float64(y)))), true, nil
	}
	return nil, false, nil
}

// goUnsignedArith is goSignedArith over unsigned bits, where every operation
// wraps at the width: uint8(0) - 1 is uint8(255).
func goUnsignedArith(op string, w model.GoWidth, a, b any) (any, bool, error) {
	x, y := uint64(goOperandInt(a)), uint64(goOperandInt(b))
	switch op {
	case "+":
		return w.Narrow(int64(x + y)), true, nil
	case "-":
		return w.Narrow(int64(x - y)), true, nil
	case "*":
		return w.Narrow(int64(x * y)), true, nil
	case "/":
		if y == 0 {
			return w.Zero(), true, nil
		}
		return w.Narrow(int64(x / y)), true, nil
	case "%":
		if y == 0 {
			return w.Zero(), true, nil
		}
		return w.Narrow(int64(x % y)), true, nil
	case "**":
		return w.Narrow(phpval.ToInt64(math.Pow(float64(x), float64(y)))), true, nil
	}
	return nil, false, nil
}

// goFloatArith computes at a float width, in a float64 that narrows back: a
// float32 operation answers the float32 go would.
func goFloatArith(op string, w model.GoWidth, a, b any) (any, bool, error) {
	x, y := goOperandFloat(a), goOperandFloat(b)
	switch op {
	case "+":
		return w.Narrow(x + y), true, nil
	case "-":
		return w.Narrow(x - y), true, nil
	case "*":
		return w.Narrow(x * y), true, nil
	case "/":
		if y == 0 {
			return w.Zero(), true, nil
		}
		return w.Narrow(x / y), true, nil
	case "%":
		// `%` is php's: both operands cast to int, which is phpval's reduction
		// and not go's undefined conversion of an out-of-range float.
		left, right := phpval.ToInt64(x), phpval.ToInt64(y)
		if right == 0 {
			return w.Zero(), true, nil
		}
		return w.Narrow(float64(left % right)), true, nil
	case "**":
		return w.Narrow(math.Pow(x, y)), true, nil
	}
	return nil, false, nil
}

// shiftLeft shifts within the int64 carrier, which the result narrows out of.
// A count of 64 or more leaves zero, as go's shift does.
func shiftLeft(x, count int64) int64 {
	if count >= 64 {
		return 0
	}
	return x << uint(count)
}

// shiftRight shifts the way the width's sign reads: an unsigned width shifts
// zeroes in, a signed one keeps its sign.
func shiftRight(w model.GoWidth, x, count int64) int64 {
	if count >= 64 {
		if w.Signed && x < 0 {
			return -1
		}
		return 0
	}
	if w.Signed {
		return x >> uint(count)
	}
	return int64(uint64(x) >> uint(count))
}

// goNegate negates at the operand's width: -int8(-128) is int8(-128), the
// wrap go answers with.
func goNegate(v any) (any, bool) {
	w, typed := model.GoWidthOf(v)
	if !typed {
		return nil, false
	}
	if w.Float {
		return w.Narrow(-goOperandFloat(v)), true
	}
	return w.Narrow(-goOperandInt(v)), true
}

// goBitNot inverts the bits at the operand's width: ~uint8(0) is uint8(255).
// A float width carries no bits and php's own cast answers for it.
func goBitNot(v any) (any, bool) {
	w, typed := model.GoWidthOf(v)
	if !typed || w.Float {
		return nil, false
	}
	return w.Narrow(^goOperandInt(v)), true
}
