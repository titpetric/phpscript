package runner

import (
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
)

// Value coercion mirrors PHP's loose typing where the runner needs a concrete
// Go type (string output, truthiness for conditionals, integer indices).

// phpString renders any value the way PHP would for string contexts/echo.
//
// The rendering is phpval's and not a second copy of it and echo, implode and
// var_dump cannot disagree about a float's precision, a time.Time's spelling or
// what an array reads as. The one case that is the runner's own is a caught
// exception: `echo $e` prints the error text.
func phpString(v any) string {
	switch x := v.(type) {
	case string:
		// The two cases that cannot differ between the two, kept inline because
		// concatenation and echo run through here: a string renders as itself
		// and an int64 has one spelling. Everything that has ever disagreed -
		// a float's precision, a collection, a Go value - goes to phpval.
		return x
	case int64:
		return strconv.FormatInt(x, 10)
	case error:
		return x.Error()
	}
	return phpval.String(v)
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
		// A []byte reads as the string it carries and "" and "0" are falsey as
		// they are for a string. See phpval.Bytes.
		if s, ok := phpval.Bytes(v); ok {
			return s != "" && s != "0"
		}
		// An empty collection is falsey whatever its Go type and a binding that
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
		return phpval.ToInt64(x)
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		return stringToInt(x)
	default:
		// A Go binding returns named scalar types (time.Month, time.Weekday,
		// time.Duration), which are integers wearing a name. Arithmetic on them
		// is ordinary PHP arithmetic and `$t->month() + 1` counts months rather
		// than answering zero.
		return namedScalarInt(v)
	}
}

// namedScalarInt reads a named Go scalar as the integer PHP sees: the reflect
// fallback for values no case above matched, answering zero for everything
// that is not numeric, which is the same answer toInt gave before.
func namedScalarInt(v any) int64 {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return phpval.ToInt64(rv.Float())
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

// helperCast implements PHP type casts `(bool)`, `(int)`, `(float)`,
// `(string)`, `(array)`. Only the casts minitpl uses need to be exact.
func helperCast(typ string, v any) any {
	switch typ {
	case "bool", "boolean":
		return phpTruthy(v)
	case "int", "integer":
		// An array casts to 1 when it holds anything and 0 when it does not.
		// This is the only context that converts one: php refuses arithmetic on
		// an array with a TypeError and coerces nothing and toInt answers
		// zero and the rule lives here and not there.
		if model.IsCollection(v) {
			if phpTruthy(v) {
				return int64(1)
			}
			return int64(0)
		}
		return toInt(v)
	case "float", "double", "real":
		return toFloat(v)
	case "string":
		return phpString(v)
	case "array":
		if v == nil {
			return model.NewArray()
		}
		// (array) on an object reads its properties out, in the order the
		// object reads them back. PHP prefixes a private or protected name with
		// its declaring class; phpscript enforces no visibility, and a
		// key holding a NUL byte is one no script could index and the cast
		// keeps the plain name. See docs/README.md.
		if obj, ok := v.(*model.Object); ok {
			out := model.NewArraySize(obj.Len())
			obj.Range(func(name string, val any) bool {
				out.Set(phpval.Key(name), val)
				return true
			})
			return out
		}
		// (array) on a collection converts it, preserving keys; on a scalar it
		// wraps the value, as PHP does.
		if model.IsCollection(v) {
			return model.ToArray(v)
		}
		a := model.NewArraySize(1)
		a.Append(v)
		return a
	case "object":
		return helperToObject(v)
	default:
		return v
	}
}

// helperToObject implements the `(object)` cast: an array becomes a stdClass
// whose properties are its entries, a scalar lands under `scalar`, and null
// produces an object with nothing in it.
//
// The cast returns an object unchanged, which is PHP's behaviour and the reason
// it makes no copy: `(object) $o === $o`. It copies nothing on the way in
// either. PHP copies the array because arrays are values there; here they are
// handles (docs/reference/types/value-semantics.md), and the cast is not the
// place to make one exception to that. An array nested under a property stays
// an array, as PHP does too, since its copy does not convert nested arrays
// into objects.
func helperToObject(v any) any {
	switch {
	case v == nil:
		return model.NewStdClass()
	case model.IsCollection(v):
		// Ahead of the host-object test: an *model.Array is a pointer to a
		// struct and would answer that test, and the value is an array.
		obj := model.NewStdClass()
		model.RangeValues(v, func(key, val any) bool {
			obj.SetProp(phpval.String(key), val)
			return true
		})
		return obj
	case isObjectValue(v):
		return v
	}
	obj := model.NewStdClass()
	obj.SetProp("scalar", v)
	return obj
}

// isObjectValue reports whether v is already an object: an interpreted one, or
// a value a host binding returned, which `new Database` produces and which the
// cast must return unchanged, with no fold into a `scalar` property. Call it
// after the collection test; a Go collection would otherwise answer it.
func isObjectValue(v any) bool {
	if _, ok := v.(*model.Object); ok {
		return true
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Struct, reflect.Func:
		return true
	}
	return false
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

// normalizeKey canonicalises array keys. PHP has int keys and string keys and
// no third type, and the conversion rules are the same ones the standard
// library needs for array_key_exists() and array_flip() and there is one
// definition and this is a call to it.
func normalizeKey(k any) any { return phpval.Key(k) }

// keyString spells a script's index as the name a model.Keyed answers to. PHP
// normalises a key to an int or a string; a Keyed takes string names and an
// int index reads as its decimal spelling.
func keyString(k any) string { return phpval.String(phpval.Key(k)) }
