package phpval

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// Compare is PHP 8's comparison operator (<=>), which sort() and rsort()
// order by. Its rules, in the order they are applied:
//
//	arrays          the shorter array is smaller, an array beats any scalar
//	null vs string  the null becomes "" and the two compare as strings
//	bool or null    both sides are cast to bool
//	numbers         two numbers, or a number and a numeric string, or two
//	                numeric strings, compare numerically, so "9" is less than
//	                "10", and a list of digits out of explode() sorts the way
//	                the same list sorts in PHP
//	anything else   both sides are cast to string and compared bytewise, which
//	                is where a number meeting a non-numeric string ends up
func Compare(x, y any) int {
	xa, xIsArray := comparableArray(x)
	ya, yIsArray := comparableArray(y)
	switch {
	case xIsArray && yIsArray:
		return compareArrays(xa, ya)
	case xIsArray:
		return 1
	case yIsArray:
		return -1
	}

	// null against a string is the one pairing PHP does not decide on
	// truthiness: null is cast to "", so null is less than "a" but equal to "".
	if x == nil {
		if s, ok := y.(string); ok {
			return strings.Compare("", s)
		}
	}
	if y == nil {
		if s, ok := x.(string); ok {
			return strings.Compare(s, "")
		}
	}
	if isBoolish(x) || isBoolish(y) {
		return compareBools(toBool(x), toBool(y))
	}

	xn, xIsNum := phpNumeric(x)
	yn, yIsNum := phpNumeric(y)
	if xIsNum && yIsNum {
		return compareNumbers(xn, yn)
	}
	return strings.Compare(String(x), String(y))
}

// comparableArray reports whether v is an array for comparison purposes,
// returning it as an *Array. A binding that hands back a native Go collection
// is one too, so it sorts where a *model.Array would.
func comparableArray(v any) (*model.Array, bool) {
	switch x := v.(type) {
	case *model.Array:
		return x, x != nil
	case nil, bool, int, int64, float64, string:
		// The values a sort sees most of the time, answered before
		// model.IsCollection reaches for reflect.
		return nil, false
	}
	if model.IsCollection(v) {
		return model.ToArray(v), true
	}
	return nil, false
}

// compareArrays orders two arrays the way PHP does: the one with fewer members
// is smaller, and same-size arrays compare member by member over the left
// array's keys. A key the right array does not have makes the left array
// greater, PHP's answer for two arrays it cannot compare.
func compareArrays(x, y *model.Array) int {
	if x.Len() != y.Len() {
		return compareBools(x.Len() > y.Len(), y.Len() > x.Len())
	}
	result := 0
	x.Range(func(key, val any) bool {
		other, ok := y.Get(key)
		if !ok {
			result = 1
			return false
		}
		result = Compare(val, other)
		return result == 0
	})
	return result
}

// isBoolish reports whether v drags the comparison into the boolean domain,
// where PHP compares (bool)$x with (bool)$y whatever the other side is.
func isBoolish(v any) bool {
	if v == nil {
		return true
	}
	_, ok := v.(bool)
	return ok
}

// toBool is PHP's truthiness: "", "0", 0 and 0.0 are false, other scalars true.
// Arrays never reach it; Compare has taken them by then.
func toBool(v any) bool {
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
	default:
		return true
	}
}

func compareBools(x, y bool) int {
	switch {
	case x == y:
		return 0
	case y:
		return -1
	default:
		return 1
	}
}

// phpNum is a value in PHP's numeric domain: an int64 when it is an integer
// that fits one, a float64 otherwise. text is the numeric string it was read
// from, kept for the precision fallback in compareNumbers.
type phpNum struct {
	i       int64
	f       float64
	isInt   bool
	text    string // source text, when the value was read from a string
	intText bool   // that text was written as a plain integer
}

func (n phpNum) float() float64 {
	if n.isInt {
		return float64(n.i)
	}
	return n.f
}

// phpNumeric returns v as a number, reporting false for anything PHP would
// compare as a string. A numeric string is a number here, which is the whole
// point: PHP compares "10" and "9" as 10 and 9.
func phpNumeric(v any) (phpNum, bool) {
	switch x := v.(type) {
	case int:
		return phpNum{i: int64(x), isInt: true}, true
	case int64:
		return phpNum{i: x, isInt: true}, true
	case float64:
		return phpNum{f: x}, true
	case string:
		return numericString(x)
	}
	return phpNum{}, false
}

// numericString reads s the way PHP's is_numeric_string does: whitespace around
// an optional sign, digits with an optional fraction and an optional exponent.
// The syntax is checked here rather than left to strconv, which also accepts
// hex floats, digit separators, "Inf" and "NaN", none of which PHP considers
// numeric, so "0x1A" has to sort as a string.
func numericString(s string) (phpNum, bool) {
	text := strings.Trim(s, " \t\n\r\v\f")
	isInt, ok := scanNumeric(text)
	if !ok {
		return phpNum{}, false
	}
	if isInt {
		if i, err := strconv.ParseInt(text, 10, 64); err == nil {
			return phpNum{i: i, isInt: true, text: text, intText: true}, true
		}
		f, _ := strconv.ParseFloat(text, 64)
		return phpNum{f: f, text: text, intText: true}, true
	}
	// An exponent out of float64's range yields ±Inf with ErrRange, which is
	// the value PHP reads for "1e400" too.
	f, err := strconv.ParseFloat(text, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return phpNum{}, false
	}
	return phpNum{f: f, text: text}, true
}

// scanNumeric validates a trimmed numeric literal, reporting whether it is one
// and whether it is written as a plain integer (no fraction, no exponent).
func scanNumeric(s string) (isInt, ok bool) {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digits := 0
	for ; i < len(s) && isDigit(s[i]); i++ {
		digits++
	}
	isInt = true
	if i < len(s) && s[i] == '.' {
		isInt = false
		for i++; i < len(s) && isDigit(s[i]); i++ {
			digits++
		}
	}
	if digits == 0 {
		return false, false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		isInt = false
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		exponent := 0
		for ; i < len(s) && isDigit(s[i]); i++ {
			exponent++
		}
		if exponent == 0 {
			return false, false
		}
	}
	return isInt, i == len(s)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// compareNumbers compares two values from the numeric domain. Two integers
// compare as int64 so that ids near 2^63 do not lose their last digits to a
// float64.
func compareNumbers(x, y phpNum) int {
	if x.isInt && y.isInt {
		return compareBools(x.i > y.i, y.i > x.i)
	}
	xf, yf := x.float(), y.float()
	if xf != yf {
		return compareBools(xf > yf, yf > xf)
	}
	// Equal as float64 is not always equal to PHP: two integer strings past
	// int64, and two exponents past float64, round to the same value, and PHP
	// compares their digits instead. It only does so for two strings, so
	// "9223372036854775808" stays equal to the int 9223372036854775807, and
	// only for two integers, so "1e20" stays equal to "100000000000000000000".
	if x.text == "" || y.text == "" {
		return 0
	}
	if (x.intText && y.intText && !(x.isInt && y.isInt)) || math.IsInf(xf, 0) {
		return strings.Compare(x.text, y.text)
	}
	return 0
}
