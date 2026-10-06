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

// GoString renders a value that is none of PHP's own scalars: something a Go
// binding returned or a database row scanned. The final return reports whether
// it knew how.
//
// A time.Time takes time.DateTime rather than Go's String or RFC3339. PHP has
// no string form for a date to copy: `echo $dateTime` is a fatal Error. The
// rule comes from where PHP writes one itself. The `date` field of var_dump,
// print_r and json_encode is Y-m-d H:i:s, and PDO hands back a DATETIME column
// as the text it was stored as. Anything else that can spell itself does, so a
// Duration is "1h30m0s" and a Month is "August".
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

// Bytes answers the string a []byte carries, and whether v was one.
//
// A PHP string is a byte string, so a binding's []byte is one of those rather
// than a list of integers: regexp.Regexp.Find returns the text it matched, not
// fifteen numbers. This is the test the value rules ask wherever a []byte would
// otherwise fall through to the reflect path and read as a slice - truthiness,
// identity, an offset read, gettype and model.IsCollection.
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
		// strconv formats in place; fmt.Sprintf would box x into an any
		// (an allocation of its own for values over 255) and run the
		// formatter. String sits under implode, str_replace, in_array and
		// array_unique, so it is worth the two lines.
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case float64:
		return FloatString(x)
	default:
		if s, ok := GoString(x); ok {
			return s
		}
		// PHP spells every array "Array" in a string context, whatever is in
		// it, and warns. A value with no string form at all - an object, there
		// being no __toString here - is the empty string; StringContext is the
		// one that reports it as the Error PHP raises.
		if model.IsCollection(v) {
			return "Array"
		}
		return ""
	}
}

// StringContext is String with the refusal PHP raises where a script's own
// conversion reaches a value that has no string form.
//
// PHP distinguishes two situations and so does this: an array key and var_dump
// never refuse a value, and a string context does. There is no __toString in
// this runtime, so an object has no string form and php's Error is the whole of
// the case. The renderer is String's, not a second one.
func StringContext(v any) (string, error) {
	if _, ok := v.(*model.Object); ok {
		return "", &ConversionError{Class: classNameOf(v)}
	}
	return String(v), nil
}

// ConversionError reports a value a string context cannot convert, under the
// class PHP raises for it: an object with no __toString. It names that class, so
// `catch (Error $e)` matches it and `catch (Exception $e)` does not, as in PHP.
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

// classNameOf answers the class an object was declared as, for the conversion
// error's message.
func classNameOf(v any) string {
	object, ok := v.(*model.Object)
	if !ok || object == nil || object.Class == nil {
		return ""
	}
	return object.Class.Name
}

// FloatString renders a float the way PHP's echo does: precision=14 significant
// digits, so 0.1*0.2 echoes as 0.02, not the round-tripping
// 0.020000000000000004. PHP's exponent form differs from Go's: the mantissa
// always carries a decimal point and the exponent has no leading zero, so 1e20
// echoes as 1.0E+20, not 1E+20 or 1e+20.
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

// Int reads v as PHP's (int) cast does: the leading numeric prefix of a string,
// zero for what has none.
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
		// A collection answers zero here, which is what the numeric context
		// wants: php refuses arithmetic on an array with a TypeError rather than
		// coercing it, so this value never reaches an operator as a number. The
		// explicit (int) cast is the one context that converts one, and it does
		// so in runner.helperCast.
		return 0
	}
}

// Float is PHP's float cast. It reads the same leading numeric prefix Int does,
// so "12abc" is 12 to both of them and "-3.5" is -3.5 here and -3 there. A
// string with no numeric prefix, and every value that is not a scalar, is 0.
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
		return 0
	}
}

// leadingFloat reads the numeric prefix of s: leading whitespace, an optional
// sign, digits, an optional fraction and an optional exponent, stopping at the
// first character that is not part of one ("12abc" is 12, "1.5x" is 1.5, ".5"
// is 0.5, "1e3" is 1000, "abc" is 0).
//
// Int, Float and Number all read the string through here, so no two of them can
// take a different number out of it, which is what this package exists to
// prevent. The prefix is the one runner.numericPrefix reads for a cast, so
// (int)"1e3" and phpval.Int("1e3") agree as well.
//
// isInt reports that the prefix was written without a fraction or an exponent,
// which is the case Number hands back an int64 for.
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
		// A lone '.' is not a fraction: "abc.def" and "." have no prefix at
		// all, and "5." keeps its digit either way.
		if digits > 0 || fraction > 0 {
			i, digits, isInt = end, digits+fraction, false
		}
	}
	if digits == 0 {
		return "", true
	}
	// An exponent takes the prefix with it, and makes the value a float:
	// PHP reads "1e3" as 1000.0 and (int)"1e3" as 1000.
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

// parseInt reads an integer prefix, saturating at the int64 bounds the way PHP
// does: (int)"99999999999999999999" is PHP_INT_MAX rather than 0.
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

// parseFloat reads a float prefix. Digits past float64's range yield +Inf or
// -Inf with ErrRange, which is the value PHP reads for them too, so the error
// is not consulted.
func parseFloat(prefix string) float64 {
	f, _ := strconv.ParseFloat(prefix, 64)
	return f
}

// Number returns v in PHP's numeric domain: an int64 for what PHP treats as an
// integer, a float64 for what it treats as a float. It is what lets abs(),
// min(), max() and array_sum() hand back the type they were given, so that
// abs(-1) is int(1) and abs(-1.5) is float(1.5). A string is read through the
// same numeric prefix Int and Float read, so the three never disagree: "12abc"
// is int64(12), "-3.5" is float64(-3.5) and "abc" is int64(0).
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
			// An integer literal past int64 is a float in PHP too:
			// "99999999999999999999" + 0 is float(1.0E+20).
			if n, err := strconv.ParseInt(prefix, 10, 64); err == nil {
				return n
			}
			return parseFloat(prefix)
		}
		return parseFloat(prefix)
	default:
		// int, int64, bool, null and everything else PHP counts as an
		// integer, which Int already spells out.
		return Int(v)
	}
}

// Key normalises an array key the way PHP does: only int and string keys
// exist, null is "", a bool is 1 or 0, a float truncates toward zero, and a
// canonical decimal string is its number - "08" and "+1" are not, see
// NumericKey.
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
		// v, not x: returning the typed value would box it again, an
		// allocation for every int64 key outside the small-int cache.
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
		return v
	}
}

// floatKey truncates toward zero. Go leaves an out-of-range float-to-int
// conversion undefined, so the ends are named rather than left to the compiler.
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

// NumericKey reports whether s is the canonical spelling of an integer, and so
// becomes an int key. The string has to read back identically from the integer
// it would become, which rules out "08", "+1", "-0", " 1", "1.0", "1e2" and
// anything past int64. Each stays a string key, so $a["08"] and $a[8] are two
// entries. ParseInt accepts most of them, hence the hand-rolled check.
func NumericKey(s string) (int64, bool) {
	digits := s
	if strings.HasPrefix(digits, "-") {
		digits = digits[1:]
		// "-0" prints as "0", so it is not the canonical spelling of anything.
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
		// Out of int64 range. PHP keeps it as a string rather than saturating.
		return 0, false
	}
	return n, true
}

// Truthy reads v the way `if ($v)` does: "" and "0" are false where "0.0" and
// "00" are true, and an empty collection is false.
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
		if n, ok := model.LenValues(v); ok {
			return n > 0
		}
		return true
	}
}
