package runner

import (
	"strconv"

	"github.com/titpetric/phpscript/model"
)

// Value coercion mirrors PHP's loose typing where the runner needs a concrete
// Go type (string output, truthiness for conditionals, integer indices).

// phpString renders any value the way PHP would for string contexts/echo.
func phpString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "1"
		}
		return ""
	case int:
		return strconv.FormatInt(int64(x), 10)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case error:
		// A caught exception ($e in catch) renders as its message, so
		// `echo $e` prints the error text.
		return x.Error()
	default:
		return ""
	}
}

// phpTruthy implements PHP's notion of truthiness for if/while/ternary.
func phpTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != "" && x != "0"
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	case *model.Array:
		return x.Len() > 0
	default:
		// An empty collection is falsey whatever its Go type, so a binding that
		// returns a []string behaves like one that returns an *Array in
		// `if ($rows)` and `empty($rows)`.
		if n, ok := model.LenValues(v); ok {
			return n > 0
		}
		return true
	}
}

// toInt coerces a value to int64 for use as an array/slice index.
func toInt(v any) int64 {
	switch x := v.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case float64:
		return int64(x)
	case string:
		i, _ := strconv.ParseInt(x, 10, 64)
		return i
	default:
		return 0
	}
}

// phpLooseEqual approximates PHP's `==` for switch/case matching: numeric
// operands compare numerically, everything else by string value.
func phpLooseEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if isNumeric(a) && isNumeric(b) {
		return toFloat(a) == toFloat(b)
	}
	return phpString(a) == phpString(b)
}

func isNumeric(v any) bool {
	switch v.(type) {
	case int, int64, float64:
		return true
	}
	return false
}

// helperCast implements PHP type casts `(bool)`, `(int)`, `(float)`,
// `(string)`, `(array)`. Only the casts minitpl uses need to be exact.
func helperCast(typ string, v any) any {
	switch typ {
	case "bool", "boolean":
		return phpTruthy(v)
	case "int", "integer":
		return toInt(v)
	case "float", "double", "real":
		return toFloat(v)
	case "string":
		return phpString(v)
	case "array":
		if v == nil {
			return model.NewArray()
		}
		// (array) on a collection converts it, preserving keys; on a scalar it
		// wraps the value, as PHP does.
		if model.IsCollection(v) {
			return model.ToArray(v)
		}
		a := model.NewArraySize(1)
		a.Append(v)
		return a
	default:
		return v
	}
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
	x, y := toInt(a), toInt(b)
	switch op {
	case "+":
		return x + y
	case "-":
		return x - y
	case "*":
		return x * y
	case "/":
		if y == 0 {
			return int64(0)
		}
		return x / y
	case "%":
		if y == 0 {
			return int64(0)
		}
		return x % y
	default:
		return int64(0)
	}
}

// unionArrays implements PHP's array `+`. The left operand wins every key it
// has; the right contributes only what is missing. Insertion order follows the
// left array and then the surviving entries of the right, which is what PHP's
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

// toFloat coerces a value to float64.
func toFloat(v any) float64 {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case float64:
		return x
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	default:
		return 0
	}
}

// normalizeKey canonicalises array keys: PHP uses int keys for integers and
// numeric strings, string keys otherwise.
func normalizeKey(k any) any {
	switch x := k.(type) {
	case int:
		return int64(x)
	case int64:
		return x
	case string:
		if i, err := strconv.ParseInt(x, 10, 64); err == nil {
			return i
		}
		return x
	default:
		return x
	}
}
