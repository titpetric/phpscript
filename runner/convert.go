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
		if a, ok := v.(*model.Array); ok {
			return a
		}
		a := model.NewArray()
		if v != nil {
			a.Append(v)
		}
		return a
	default:
		return v
	}
}

// phpArith applies + - * / % with the same int coercion used by += and -=.
func phpArith(op string, a, b any) any {
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
