package phpval

import (
	"math"
)

// Increment applies PHP's ++ operator: numbers and numeric strings step by
// one (an int64 that would overflow becomes a float, as PHP's do), null
// becomes int 1, and a non-numeric string takes the Perl-style alphanumeric
// increment. Booleans, and any type PHP only warns about, are unchanged.
func Increment(v any) any {
	if num, ok := phpNumeric(v); ok {
		if num.isInt {
			if num.i == math.MaxInt64 {
				return float64(num.i) + 1
			}
			return num.i + 1
		}
		return num.f + 1
	}
	switch x := v.(type) {
	case nil:
		return int64(1)
	case string:
		return incrementString(x)
	}
	return v
}

// Decrement applies PHP's -- operator: numbers and numeric strings step
// down by one (an int64 that would overflow becomes a float). There is no
// string decrement; a non-numeric string is unchanged, except the empty
// string, which PHP reads as 0 and returns int -1 for. Null and booleans
// are unchanged.
func Decrement(v any) any {
	if num, ok := phpNumeric(v); ok {
		if num.isInt {
			if num.i == math.MinInt64 {
				return float64(num.i) - 1
			}
			return num.i - 1
		}
		return num.f - 1
	}
	if s, ok := v.(string); ok && s == "" {
		return int64(-1)
	}
	return v
}

// StringNumber returns the number a numeric string denotes, as int64 or
// float64 by the string's own spelling, reporting false for anything
// is_numeric rejects.
func StringNumber(s string) (any, bool) {
	num, ok := numericString(s)
	if !ok {
		return nil, false
	}
	if num.isInt {
		return num.i, true
	}
	return num.f, true
}

// incrementString is PHP's increment_string: the last character steps
// within its class (a-z, A-Z, 0-9), z, Z and 9 wrap and carry left. A carry
// that reaches a character outside the three classes is discarded; one that
// runs off the start of the string prepends a, A or 1 by the class of the
// first character. The empty string increments to "1".
func incrementString(s string) string {
	if s == "" {
		return "1"
	}
	b := []byte(s)
	for i := len(b) - 1; i >= 0; i-- {
		switch c := b[i]; {
		case c == 'z':
			b[i] = 'a'
		case c == 'Z':
			b[i] = 'A'
		case c == '9':
			b[i] = '0'
		case c >= 'a' && c < 'z', c >= 'A' && c < 'Z', c >= '0' && c < '9':
			b[i] = c + 1
			return string(b)
		default:
			return string(b)
		}
	}
	switch {
	case b[0] == 'a':
		return "a" + string(b)
	case b[0] == 'A':
		return "A" + string(b)
	}
	return "1" + string(b)
}
