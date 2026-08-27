package runner

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

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
		return phpFloatString(x)
	case error:
		// A caught exception ($e in catch) renders as its message, so
		// `echo $e` prints the error text.
		return x.Error()
	case fmt.Stringer:
		// A Go value that knows how to spell itself is echoed that way, so a
		// time.Time from a DATETIME column, a time.Duration and a time.Month
		// all print rather than vanishing. PHP has no counterpart to fall back
		// on: it refuses to convert an object to a string at all, so the Go
		// stringer is the only sensible answer and an empty one is a bug.
		return x.String()
	default:
		return ""
	}
}

// phpFloatString renders a float the way PHP's echo does: precision=14
// significant digits, so 0.1*0.2 echoes as 0.02, not the round-tripping
// 0.020000000000000004. PHP's exponent form differs from Go's: the mantissa
// always carries a decimal point and the exponent has no leading zero, so
// 1e20 echoes as 1.0E+20, not 1E+20 or 1e+20.
func phpFloatString(x float64) string {
	switch {
	case math.IsInf(x, 1):
		return "INF"
	case math.IsInf(x, -1):
		return "-INF"
	case math.IsNaN(x):
		return "NAN"
	}
	s := strconv.FormatFloat(x, 'G', 14, 64)
	if i := strings.IndexByte(s, 'E'); i >= 0 {
		mant, exp := s[:i], s[i+1:]
		if !strings.Contains(mant, ".") {
			mant += ".0"
		}
		sign := ""
		if exp != "" && (exp[0] == '+' || exp[0] == '-') {
			sign, exp = exp[:1], exp[1:]
		}
		if trimmed := strings.TrimLeft(exp, "0"); trimmed != "" {
			exp = trimmed
		}
		s = mant + "E" + sign + exp
	}
	return s
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
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		return stringToInt(x)
	default:
		// A Go binding hands back named scalar types (time.Month, time.Weekday,
		// time.Duration), which are integers wearing a name. Arithmetic on them
		// is ordinary PHP arithmetic, so `$t->month() + 1` counts months rather
		// than answering zero.
		return namedScalarInt(v)
	}
}

// namedScalarInt reads a named Go scalar as the integer PHP sees. It is the
// reflect fallback for values no case above matched, and answers zero for
// everything that is not numeric, which is the same answer toInt gave before.
func namedScalarInt(v any) int64 {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return int64(rv.Float())
	}
	return 0
}

// numericPrefix returns the longest leading substring of s that reads as a
// PHP number, and whether it uses float syntax. PHP's string-to-number
// coercion takes this prefix and ignores the rest: (int)"12abc" is 12,
// (float)"2.5kg" is 2.5. An empty prefix means the string has no leading
// number.
func numericPrefix(s string) (prefix string, isFloat bool) {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' || s[i] == '\v' || s[i] == '\f') {
		i++
	}
	start := i
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	intDigits := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
		intDigits++
	}
	end := i
	fracDigits := 0
	if i < len(s) && s[i] == '.' {
		j := i + 1
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
			fracDigits++
		}
		if intDigits > 0 || fracDigits > 0 {
			i, end, isFloat = j, j, true
		}
	}
	if intDigits+fracDigits == 0 {
		return "", false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		expDigits := 0
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
			expDigits++
		}
		if expDigits > 0 {
			end, isFloat = j, true
		}
	}
	return s[start:end], isFloat
}

// stringToInt implements PHP's string-to-int coercion: the integer value of
// the leading number, with float syntax truncated toward zero and an
// out-of-range integer saturated the way C's strtol (which PHP's cast uses)
// saturates.
func stringToInt(s string) int64 {
	prefix, isFloat := numericPrefix(s)
	if prefix == "" {
		return 0
	}
	if !isFloat {
		i, err := strconv.ParseInt(prefix, 10, 64)
		if err == nil {
			return i
		}
		if strings.HasPrefix(prefix, "-") {
			return math.MinInt64
		}
		return math.MaxInt64
	}
	f, _ := strconv.ParseFloat(prefix, 64)
	return int64(f)
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
	// Integer arithmetic that overflows becomes float in PHP, so
	// PHP_INT_MAX + 1 is 9.2233720368548E+18, not a wrapped negative.
	x, y := toInt(a), toInt(b)
	switch op {
	case "+":
		if z, ok := addInt(x, y); ok {
			return z
		}
		return float64(x) + float64(y)
	case "-":
		if z, ok := subInt(x, y); ok {
			return z
		}
		return float64(x) - float64(y)
	case "*":
		if z, ok := mulInt(x, y); ok {
			return z
		}
		return float64(x) * float64(y)
	case "/":
		if y == 0 {
			return int64(0)
		}
		// Integer division that does not divide evenly is float in PHP.
		if x%y != 0 {
			return float64(x) / float64(y)
		}
		return x / y
	default:
		return int64(0)
	}
}

// phpBitwise applies & | ^ << >>.
//
// Both operands are cast to int, except that & | ^ between two strings operate
// bytewise and yield a string, as they do in PHP: "a" | "b" is "c". The operand
// length follows PHP: `&` and `^` stop at the shorter operand, `|` keeps the
// longer one and treats the missing bytes as zero.
//
// Shifts are int64 operations. A count of 64 or more needs no special case: Go
// defines an over-wide shift as 0 for `<<`, and 0 or -1 by sign for `>>`, which
// is what PHP produces. A negative count is the one input with no answer, and
// PHP raises ArithmeticError for it rather than picking one.
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
// A float is negated directly rather than computed as `0 - x`, which loses the
// sign of zero: PHP echoes -0.0 as -0. Everything else goes through the
// arithmetic helper so that negating the smallest int64 overflows to a float
// the way every other arithmetic operator does, instead of wrapping back to
// itself with Go's int64 negation.
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
// those the way every other bitwise operand is cast, so `~null` is -1 rather
// than a fatal error (docs/README.md).
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

// addInt, subInt and mulInt perform int64 arithmetic, reporting false on
// overflow so phpArith can fall back to float the way PHP does.
func addInt(x, y int64) (int64, bool) {
	z := x + y
	if (y > 0 && z < x) || (y < 0 && z > x) {
		return 0, false
	}
	return z, true
}

func subInt(x, y int64) (int64, bool) {
	z := x - y
	if (y < 0 && z < x) || (y > 0 && z > x) {
		return 0, false
	}
	return z, true
}

func mulInt(x, y int64) (int64, bool) {
	z := x * y
	if x != 0 && (z/x != y || (x == -1 && y == math.MinInt64)) {
		return 0, false
	}
	return z, true
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
			r, ok := mulInt(result, sq)
			if !ok {
				return math.Pow(float64(base), float64(exp))
			}
			result = r
		}
		if e > 1 {
			s, ok := mulInt(sq, sq)
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
		prefix, _ := numericPrefix(x)
		if prefix == "" {
			return 0
		}
		f, _ := strconv.ParseFloat(prefix, 64)
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
