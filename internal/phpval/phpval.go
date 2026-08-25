// Package phpval holds PHP's value semantics: how a value converts to a
// string, an integer or a truth value, how a collection reads back as a list
// of values or strings, and how two values order under PHP 8's <=> operator.
//
// It exists so the files under stdlib/core can each register their own area
// without importing one another. Every one of them coerces its arguments, and
// the coercion has to have exactly one definition: sort() and in_array()
// disagreeing about what "10" is would be a bug no test names.
package phpval

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/titpetric/phpscript/model"
)

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
		// %v for a float64 is 'g' with the shortest representation that
		// round-trips, which is exactly what FormatFloat(-1) produces.
		return strconv.FormatFloat(x, 'g', -1, 64)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func Int(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
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
		return int64(parseFloat(prefix))
	default:
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

// Key normalises an array key the way the runner stores one: an int becomes an
// int64, a decimal string becomes the int64 it spells, and everything else is
// returned unchanged. array_key_exists() and array_flip() have to name a key
// the same way the assignment that created it did, so this reproduces
// runner.normalizeKey, down to "01" being the key 1.
func Key(v any) any {
	switch x := v.(type) {
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
