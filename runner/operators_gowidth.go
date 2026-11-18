package runner

import (
	"fmt"
	"math"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
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

// goArith applies an arithmetic operator at a typed width when operands have one.
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

// goBitwise applies a bitwise operator at a typed width, a long shift zero.
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

// goOperandWidth returns the operation's width, reporting whether either value has a Go width.
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

// goOperandInt returns the bits an operand computes over, converting an untyped php number.
func goOperandInt(v any) int64 {
	if bits, ok := modelutil.GoInt(v); ok {
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

// goSignedArith computes at a signed width, narrowing the int64 carrier to wrap.
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
			return nil, true, &DivisionByZeroError{Message: "Division by zero"}
		}
		// go's division truncates toward zero and keeps the type, where php's
		// `/` answers a float for an inexact one: int8(7) / int8(2) is int8(3).
		return w.Narrow(x / y), true, nil
	case "%":
		if y == 0 {
			return nil, true, &DivisionByZeroError{Message: "Modulo by zero"}
		}
		return w.Narrow(x % y), true, nil
	case "**":
		return w.Narrow(phpval.ToInt64(math.Pow(float64(x), float64(y)))), true, nil
	}
	return nil, false, nil
}

// goUnsignedArith computes at an unsigned width, wrapping every operation at it.
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
			return nil, true, &DivisionByZeroError{Message: "Division by zero"}
		}
		return w.Narrow(int64(x / y)), true, nil
	case "%":
		if y == 0 {
			return nil, true, &DivisionByZeroError{Message: "Modulo by zero"}
		}
		return w.Narrow(int64(x % y)), true, nil
	case "**":
		return w.Narrow(phpval.ToInt64(math.Pow(float64(x), float64(y)))), true, nil
	}
	return nil, false, nil
}

// goFloatArith computes in float64 and narrows back for float32 answers.
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
			return nil, true, &DivisionByZeroError{Message: "Division by zero"}
		}
		return w.Narrow(x / y), true, nil
	case "%":
		// `%` is php's: both operands cast to int, which is phpval's reduction
		// and not go's undefined conversion of an out-of-range float.
		left, right := phpval.ToInt64(x), phpval.ToInt64(y)
		if right == 0 {
			return nil, true, &DivisionByZeroError{Message: "Modulo by zero"}
		}
		return w.Narrow(float64(left % right)), true, nil
	case "**":
		return w.Narrow(math.Pow(x, y)), true, nil
	}
	return nil, false, nil
}

// shiftLeft shifts within the int64 carrier; 64 or more leaves zero.
func shiftLeft(x, count int64) int64 {
	if count >= 64 {
		return 0
	}
	return x << uint(count)
}

// shiftRight shifts in zeroes when unsigned, or keeps the sign when signed.
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

// goNegate negates at the operand's width, wrapping as go does.
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

// goBitNot inverts the bits at the operand's width, reporting false for a float.
func goBitNot(v any) (any, bool) {
	w, typed := model.GoWidthOf(v)
	if !typed || w.Float {
		return nil, false
	}
	return w.Narrow(^goOperandInt(v)), true
}
