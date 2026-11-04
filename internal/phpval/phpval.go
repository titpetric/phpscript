// Package phpval holds PHP's value semantics: how a value converts to a
// string, an integer or a truth value, how a collection reads back as a list
// of values or strings, and how two values order under PHP 8's <=> operator.
//
// It exists so the files under stdlib/core can each register their own area
// without importing one another, and so the runner agrees with them. The
// coercion needs exactly one definition: sort() and in_array() disagreeing
// about what "10" is would be a bug no test names.
package phpval

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/titpetric/phpscript/model"
)

// GoString renders a value outside php's scalars, reporting whether it has one.
func GoString(v any) (string, bool) {
	switch x := v.(type) {
	case time.Time:
		return x.Format(time.DateTime), true
	case []byte:
		return string(x), true
	case fmt.Stringer:
		return x.String(), true
	}
	return "", false
}

// Bytes answers the string a []byte carries, and whether v holds one.
func Bytes(v any) (string, bool) {
	b, ok := v.([]byte)
	if !ok {
		return "", false
	}
	return string(b), true
}

// String renders v the way PHP renders a value in a string context.
func String(v any) string {
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
	case int64:
		// strconv formats in place, where fmt.Sprintf boxes x and formats.
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case float64:
		return FloatString(x)
	case uint64:
		// An unsigned width renders its own value: widening reaches a float.
		return strconv.FormatUint(x, 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case float32:
		// go's shortest round-trip at 32 bits: float32(0.1) renders 0.1.
		return strconv.FormatFloat(float64(x), 'g', -1, 32)
	default:
		if s, ok := GoString(x); ok {
			return s
		}
		// A typed go value renders as the number it holds.
		if widened, ok := model.GoWiden(v); ok {
			return String(widened)
		}
		// php spells every array "Array" in a string context.
		if model.IsCollection(v) {
			return "Array"
		}
		// An object has no string form here, and StringContext refuses it.
		return ""
	}
}

// StringContext is String, refusing an object as php's Error does.
func StringContext(v any) (string, error) {
	if _, ok := v.(*model.Object); ok {
		return "", &ConversionError{Class: classNameOf(v)}
	}
	return String(v), nil
}

// ConversionError reports a value a string context cannot convert.
type ConversionError struct {
	Class string
}

// Error is php's wording for the same conversion.
func (e *ConversionError) Error() string {
	class := e.Class
	if class == "" {
		class = "stdClass"
	}
	return "Object of class " + class + " could not be converted to string"
}

// ThrowableClass names the PHP class, implementing runner.Throwable.
func (e *ConversionError) ThrowableClass() string { return "Error" }

// classNameOf answers the class an object declares.
func classNameOf(v any) string {
	object, ok := v.(*model.Object)
	if !ok || object == nil || object.Class == nil {
		return ""
	}
	return object.Class.Name
}

// FloatString renders a float the way php's echo does, at precision 14.
func FloatString(x float64) string {
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

// Int reads v as php's (int) cast does.
func Int(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return ToInt64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		prefix, isInt := leadingFloat(x)
		if prefix == "" {
			return 0
		}
		if isInt {
			return parseInt(prefix)
		}
		return ToInt64(parseFloat(prefix))
	default:
		// A typed integer reads as php's int, clipping at PHP_INT_MAX.
		if clipped, ok := model.GoClipInt(v); ok {
			return clipped
		}
		// Every other typed value reads as the number it holds.
		if widened, ok := model.GoWiden(v); ok {
			return Int(widened)
		}
		// A collection is zero here; runner.helperCast owns the (int) cast.
		return 0
	}
}

// Digits renders the decimal %d prints, reporting a negative value.
func Digits(v any) (string, bool) {
	// An unsigned width prints its own value, as fmt's %d does.
	if w, typed := model.GoWidthOf(v); typed && !w.Float && !w.Signed {
		bits, _ := model.GoUint(v)
		return strconv.FormatUint(bits, 10), false
	}
	n := Int(v)
	return strconv.FormatInt(n, 10), n < 0
}

// Uint reads the bits %u, %b, %o and %x print.
func Uint(v any) uint64 {
	// A typed integer answers its own bits, whatever its sign.
	if bits, typed := model.GoUint(v); typed {
		return bits
	}
	return uint64(Int(v))
}

// Float reads v as php's (float) cast does.
func Float(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int64:
		return float64(x)
	case int:
		return float64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		prefix, _ := leadingFloat(x)
		return parseFloat(prefix)
	default:
		if widened, ok := model.GoWiden(v); ok {
			return Float(widened)
		}
		return 0
	}
}

// leadingFloat reads the numeric prefix of s, reporting an integer spelling.
func leadingFloat(s string) (prefix string, isInt bool) {
	i := 0
	for i < len(s) {
		switch s[i] {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			i++
			continue
		}
		break
	}
	start := i
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digits := 0
	for ; i < len(s) && isDigit(s[i]); i++ {
		digits++
	}
	isInt = true
	if i < len(s) && s[i] == '.' {
		end, fraction := i+1, 0
		for ; end < len(s) && isDigit(s[end]); end++ {
			fraction++
		}
		// A lone '.' is no fraction, and "5." keeps its digit either way.
		if digits > 0 || fraction > 0 {
			i, digits, isInt = end, digits+fraction, false
		}
	}
	if digits == 0 {
		return "", true
	}
	// An exponent joins the prefix and makes the value a float.
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		end := i + 1
		if end < len(s) && (s[end] == '+' || s[end] == '-') {
			end++
		}
		exponent := 0
		for ; end < len(s) && isDigit(s[end]); end++ {
			exponent++
		}
		if exponent > 0 {
			i, isInt = end, false
		}
	}
	return s[start:i], isInt
}

// parseInt reads an integer prefix, saturating at the int64 bounds as php does.
func parseInt(prefix string) int64 {
	n, err := strconv.ParseInt(prefix, 10, 64)
	if err == nil {
		return n
	}
	if strings.HasPrefix(prefix, "-") {
		return math.MinInt64
	}
	return math.MaxInt64
}

// parseFloat reads a float prefix, where digits past float64 reach an infinity.
func parseFloat(prefix string) float64 {
	f, _ := strconv.ParseFloat(prefix, 64)
	return f
}

// Number returns v in php's numeric domain, an int64 or a float64.
func Number(v any) any {
	switch x := v.(type) {
	case float64:
		return x
	case string:
		prefix, isInt := leadingFloat(x)
		if prefix == "" {
			return int64(0)
		}
		if isInt {
			// An integer spelling past int64 is a float, as php reads it.
			if n, err := strconv.ParseInt(prefix, 10, 64); err == nil {
				return n
			}
			return parseFloat(prefix)
		}
		return parseFloat(prefix)
	default:
		// A typed value answers the number it holds, a float past PHP_INT_MAX.
		if widened, ok := model.GoWiden(v); ok {
			return widened
		}
		// int, int64, bool, null and the rest are integers, which Int spells.
		return Int(v)
	}
}

// Key normalises an array key the way php does, to an int or a string.
func Key(v any) any {
	switch x := v.(type) {
	case nil:
		return ""
	case bool:
		if x {
			return int64(1)
		}
		return int64(0)
	case int:
		return int64(x)
	case int64:
		// v, not x: returning x boxes the value a second time.
		return v
	case float64:
		return floatKey(x)
	case float32:
		return floatKey(float64(x))
	case string:
		if i, ok := NumericKey(x); ok {
			return i
		}
		// v, not x, for the same reason as int64 above.
		return v
	default:
		// A typed value keys as the number it holds: int8(44) keys 44.
		if widened, ok := model.GoWiden(v); ok {
			return Key(widened)
		}
		return v
	}
}

// floatKey truncates toward zero, naming the ends go leaves undefined.
func floatKey(f float64) int64 {
	switch {
	case math.IsNaN(f):
		return 0
	case f >= math.MaxInt64:
		return math.MaxInt64
	case f <= math.MinInt64:
		return math.MinInt64
	}
	return int64(f)
}

// NumericKey reports whether s is the canonical spelling of an integer key.
func NumericKey(s string) (int64, bool) {
	digits := s
	if strings.HasPrefix(digits, "-") {
		digits = digits[1:]
		// "-0" prints as "0" and it never spells a canonical integer.
		if digits == "0" {
			return 0, false
		}
	}
	if digits == "" {
		return 0, false
	}
	// A leading zero is canonical only when the whole number is "0".
	if digits[0] == '0' && len(digits) > 1 {
		return 0, false
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		// Past int64, where php keeps the string key and saturates nothing.
		return 0, false
	}
	return n, true
}

// Truthy reads v the way `if ($v)` does.
func Truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != "" && x != "0"
	case int64:
		return x != 0
	case int:
		return x != 0
	case float64:
		return x != 0
	case *model.Array:
		return x.Len() > 0
	default:
		// A typed value reads as the number it holds, where int8(0) is false.
		if widened, ok := model.GoWiden(v); ok {
			return Truthy(widened)
		}
		// An empty collection is false whatever its go type.
		if n, ok := model.LenValues(v); ok {
			return n > 0
		}
		return true
	}
}
