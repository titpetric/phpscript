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
		return leadingInt(x)
	default:
		return 0
	}
}

// leadingInt reads the integer prefix of s the way fmt.Sscanf(s, "%d", &n) did:
// leading whitespace and an optional sign, then digits, stopping at the first
// character that is not one ("12abc" is 12, "abc" is 0). Overflow yields 0, the
// value Sscanf left behind when it failed. Sscanf allocated a scan state, a
// reader and an argument box on every call; this reads the string in place.
func leadingInt(s string) int64 {
	i := 0
	for i < len(s) {
		switch s[i] {
		case ' ', '\t', '\n', '\r', '\v', '\f':
			i++
			continue
		}
		break
	}
	negative := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		negative = s[i] == '-'
		i++
	}
	var n int64
	for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		digit := int64(s[i] - '0')
		if n > (math.MaxInt64-digit)/10 {
			return 0
		}
		n = n*10 + digit
	}
	if negative {
		return -n
	}
	return n
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
