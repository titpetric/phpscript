package core

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
)

// init contributes PHP's math functions to stdlib.Register.
func init() {
	runner.RegisterBinding(registerMath)
}

func registerMath(rt *runner.Runtime) {
	rt.SetConst("M_PI", math.Pi)
	rt.SetConst("M_E", math.E)

	// abs returns the absolute value of $num, an int for an int argument and a float for a float one.
	rt.RegisterFunc("abs", phpAbs)
	// floor returns the next lowest integer value of $num as a float, so floor(4.7) is float(4).
	rt.RegisterFunc("floor", func(num any) float64 { return math.Floor(phpval.Float(num)) })
	// ceil returns the next highest integer value of $num as a float, so ceil(4.3) is float(5).
	rt.RegisterFunc("ceil", func(num any) float64 { return math.Ceil(phpval.Float(num)) })
	// round returns $num rounded to $precision decimal places as a float, always half away from zero; a $mode argument is accepted and ignored, so only PHP_ROUND_HALF_UP is honoured.
	rt.RegisterFunc("round", func(num any, opts ...any) float64 {
		precision := 0
		if len(opts) > 0 {
			precision = int(phpval.Int(opts[0]))
		}
		return phpRound(phpval.Float(num), precision)
	})
	// sqrt returns the square root of $num as a float, or NAN when $num is negative.
	rt.RegisterFunc("sqrt", func(num any) float64 { return math.Sqrt(phpval.Float(num)) })
	// pow returns $num raised to the power $exponent, an int when both are int and the result fits, a float otherwise.
	rt.RegisterFunc("pow", phpPow)
	// log returns the logarithm of $num in base $base, natural (base M_E) when $base is omitted.
	rt.RegisterFunc("log", func(num any, base ...any) float64 {
		if len(base) == 0 {
			return math.Log(phpval.Float(num))
		}
		return phpLog(phpval.Float(num), phpval.Float(base[0]))
	})
	// min returns the lowest value of $value and $values, or of the single array argument; values compare as PHP 8 compares them and the value itself is returned, so min(1, "2", 3) is int(1).
	rt.RegisterFunc("min", func(args ...any) (any, error) { return phpMinMax("min", args, -1) })
	// max returns the highest value of $value and $values, or of the single array argument; values compare as PHP 8 compares them and the value itself is returned, so max(1, "2", 3) is int(3).
	rt.RegisterFunc("max", func(args ...any) (any, error) { return phpMinMax("max", args, 1) })
	// number_format formats $num with $decimals decimals, $decimal_separator between the parts and $thousands_separator every three digits of the integer part, rounding half away from zero.
	rt.RegisterFunc("number_format", phpNumberFormat)
}

// phpAbs backs abs. phpval.Number decides the return type, so an int argument
// keeps its type and a float argument stays a float. PHP_INT_MIN has no int
// counterpart and becomes a float, as it does in PHP.
func phpAbs(num any) any {
	switch x := phpval.Number(num).(type) {
	case float64:
		return math.Abs(x)
	case int64:
		if x == math.MinInt64 {
			return -float64(x)
		}
		if x < 0 {
			return -x
		}
		return x
	default:
		return int64(0)
	}
}

// phpPow backs pow. It is the same rule the ** operator follows in
// runner.phpPow: two int operands with a non-negative exponent stay int, and a
// float operand, a negative exponent or an int result that overflows makes the
// result a float. The two implementations are held together by a fixture that
// prints pow(2, 63) next to 2 ** 63.
func phpPow(num, exponent any) any {
	a, b := phpval.Number(num), phpval.Number(exponent)
	af, aIsFloat := a.(float64)
	bf, bIsFloat := b.(float64)
	if aIsFloat || bIsFloat {
		if !aIsFloat {
			af = phpval.Float(a)
		}
		if !bIsFloat {
			bf = phpval.Float(b)
		}
		return math.Pow(af, bf)
	}
	base, exp := a.(int64), b.(int64)
	if exp < 0 {
		return math.Pow(float64(base), float64(exp))
	}
	result, sq := int64(1), base
	for e := exp; e > 0; e >>= 1 {
		if e&1 == 1 {
			r, ok := mulInt64(result, sq)
			if !ok {
				return math.Pow(float64(base), float64(exp))
			}
			result = r
		}
		if e > 1 {
			s, ok := mulInt64(sq, sq)
			if !ok {
				return math.Pow(float64(base), float64(exp))
			}
			sq = s
		}
	}
	return result
}

// mulInt64 multiplies two int64s, reporting false on overflow so pow can fall
// back to a float the way PHP does.
func mulInt64(x, y int64) (int64, bool) {
	z := x * y
	if x != 0 && (z/x != y || (x == -1 && y == math.MinInt64)) {
		return 0, false
	}
	return z, true
}

// phpLog backs log with an explicit base. Bases 2 and 10 read through
// math.Log2 and math.Log10, which is what PHP does: the division of two
// logarithms is a digit out on some exact powers, and log(1024, 2) has to be
// float(10) rather than 10.000000000000002.
func phpLog(num, base float64) float64 {
	switch base {
	case 2:
		return math.Log2(num)
	case 10:
		return math.Log10(num)
	default:
		return math.Log(num) / math.Log(base)
	}
}

// phpMinMax backs min and max. Either one collection or a list of values is
// accepted, every candidate is ordered with phpval.Compare, and the winning
// element is returned as it was given, so max(1, "2", 3) is int(3) rather than
// a number the comparison produced. want is 1 for max and -1 for min.
//
// Equal values are broken the way PHP breaks them, which is not the same way
// for both: min keeps the last of several equal values and max keeps the first,
// so min(1, 1.0) is float(1) and max(1, 1.0) is int(1).
func phpMinMax(name string, args []any, want int) (any, error) {
	values := args
	if len(args) == 1 {
		if !model.IsCollection(args[0]) {
			return nil, errors.New(name + "(): Argument #1 ($value) must be of type array")
		}
		values = phpval.Values(args[0])
	}
	if len(values) == 0 {
		return nil, errors.New(name + "(): Argument #1 ($value) must contain at least one element")
	}
	best := values[0]
	for _, value := range values[1:] {
		order := phpval.Compare(value, best)
		if order == want || (order == 0 && want == -1) {
			best = value
		}
	}
	return best, nil
}

// integerDigits renders an integer argument as its own decimal digits, so that
// grouping an int64 does not lose the low bits a float64 cannot carry. exact is
// false for every other value, which then rounds through a float as before.
//
// A negative $decimals rounds the integer, so it is not a pure digit copy: the
// digits past the cut are dropped and the kept ones carry, the same rule
// roundDecimal applies to a decimal string.
func integerDigits(num any, decimals int) (neg bool, integer, fraction string, exact bool) {
	var n int64
	switch v := num.(type) {
	case int64:
		n = v
	case int:
		n = int64(v)
	default:
		return false, "", "", false
	}

	neg = n < 0
	digits := strconv.FormatInt(n, 10)
	if neg {
		digits = digits[1:]
	}

	if decimals < 0 {
		cut := len(digits) + decimals
		switch {
		case cut <= 0:
			// Every digit is dropped, so the result rounds to zero unless the
			// first dropped digit carries into a place the value does not have.
			if cut == 0 && digits[0] >= '5' {
				digits = "1" + strings.Repeat("0", len(digits))
			} else {
				digits = "0"
			}
		default:
			kept := digits[:cut]
			if digits[cut] >= '5' {
				kept = string(incrementDigits([]byte(kept)))
			}
			digits = kept + strings.Repeat("0", -decimals)
		}
		decimals = 0
	}

	fraction = strings.Repeat("0", decimals)
	if allZeroDigits(digits) {
		neg = false
	}
	return neg, digits, fraction, true
}

// phpNumberFormat backs number_format.
func phpNumberFormat(num any, opts ...any) string {
	decimals := 0
	if len(opts) > 0 {
		decimals = int(phpval.Int(opts[0]))
	}
	decimalSeparator, thousandsSeparator := ".", ","
	if len(opts) > 1 && opts[1] != nil {
		decimalSeparator = phpval.String(opts[1])
	}
	if len(opts) > 2 && opts[2] != nil {
		thousandsSeparator = phpval.String(opts[2])
	}

	// An integer is grouped from its own digits rather than through a float.
	// float64 carries 53 bits of mantissa, so routing PHP_INT_MAX through one
	// would print 9,223,372,036,854,776,000 instead of the number given.
	neg, integer, fraction, exact := integerDigits(num, decimals)
	if !exact {
		value := phpval.Float(num)
		if math.IsNaN(value) {
			return "nan"
		}
		if math.IsInf(value, 0) {
			if value < 0 {
				return "-inf"
			}
			return "inf"
		}
		neg, integer, fraction = roundDecimal(value, decimals)
	}
	// A value that rounds to zero has no sign in PHP: number_format(-0.4) is
	// "0", not "-0".
	if neg && (allZeroDigits(integer) && allZeroDigits(fraction)) {
		neg = false
	}

	var out strings.Builder
	out.Grow(len(integer) + len(integer)/3*len(thousandsSeparator) + len(fraction) + len(decimalSeparator) + 1)
	if neg {
		out.WriteByte('-')
	}
	for i := 0; i < len(integer); i++ {
		if i > 0 && (len(integer)-i)%3 == 0 {
			out.WriteString(thousandsSeparator)
		}
		out.WriteByte(integer[i])
	}
	if fraction != "" {
		out.WriteString(decimalSeparator)
		out.WriteString(fraction)
	}
	return out.String()
}

// phpRound rounds v to precision decimal places, half away from zero.
//
// The rounding is done on the decimal text rather than on the binary value.
// PHP rounds what the number prints as, and the two disagree wherever a decimal
// literal has no exact float: 1.005 is stored as 1.00499999999999989..., so
// math.Round(1.005*100)/100 is 1.0 where PHP's round(1.005, 2) is 1.01.
// Formatting with 'f' and -1 first yields the shortest decimal that reads back
// as the same float ("1.005"), which is the number the script wrote, and
// rounding that text reproduces PHP for every case. Rounding through
// FormatFloat with a precision instead would round half to even, making
// round(2.5) 2.0.
func phpRound(v float64, precision int) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	neg, integer, fraction := roundDecimal(v, precision)
	var text strings.Builder
	text.Grow(len(integer) + len(fraction) + 2)
	if neg {
		text.WriteByte('-')
	}
	text.WriteString(integer)
	if fraction != "" {
		text.WriteByte('.')
		text.WriteString(fraction)
	}
	rounded, err := strconv.ParseFloat(text.String(), 64)
	if err != nil {
		return v
	}
	return rounded
}

// roundDecimal rounds v half away from zero at precision decimal places and
// returns the result as a sign and two digit strings, the integer part and the
// fraction. The fraction is exactly precision digits long, empty when precision
// is zero or less; a negative precision rounds to the left of the point, so
// roundDecimal(1234.5678, -2) is "1200" and "".
//
// The number is taken apart as the digit string FormatFloat produced plus the
// position of its point. Dropping everything from index cut = len(integer) +
// precision and incrementing the digit before it when the first dropped digit
// is 5 or more is decimal half-away-from-zero rounding, and it needs no
// arithmetic on the float, which is the point: the float cannot represent the
// value the script wrote.
func roundDecimal(v float64, precision int) (neg bool, integer, fraction string) {
	text := strconv.FormatFloat(v, 'f', -1, 64)
	if strings.HasPrefix(text, "-") {
		neg, text = true, text[1:]
	}
	whole, frac := text, ""
	if i := strings.IndexByte(text, '.'); i >= 0 {
		whole, frac = text[:i], text[i+1:]
	}
	digits := whole + frac

	// kept holds the digits of the result scaled by 10^precision, so the value
	// is kept * 10^-precision whichever side of the point the cut fell on.
	var kept []byte
	switch cut := len(whole) + precision; {
	case cut < 0:
		// Every digit is dropped and the value is below half of the last kept
		// place, so the result is zero.
	case cut >= len(digits):
		// Nothing is dropped; pad out to the requested precision.
		kept = make([]byte, 0, cut)
		kept = append(kept, digits...)
		for len(kept) < cut {
			kept = append(kept, '0')
		}
	default:
		kept = make([]byte, cut, cut+1)
		copy(kept, digits[:cut])
		if digits[cut] >= '5' {
			kept = incrementDigits(kept)
		}
	}

	if precision >= 0 {
		// The fraction is the last precision digits, so the number needs at
		// least that many plus one for the integer part.
		for len(kept) <= precision {
			kept = append([]byte{'0'}, kept...)
		}
		split := len(kept) - precision
		return neg, string(kept[:split]), string(kept[split:])
	}
	if allZeroDigits(string(kept)) {
		return neg, "0", ""
	}
	return neg, string(kept) + strings.Repeat("0", -precision), ""
}

// incrementDigits adds one to a decimal digit string, growing it by a leading
// "1" when the carry runs off the front ("99" -> "100", "" -> "1").
func incrementDigits(digits []byte) []byte {
	for i := len(digits) - 1; i >= 0; i-- {
		if digits[i] < '9' {
			digits[i]++
			return digits
		}
		digits[i] = '0'
	}
	return append([]byte{'1'}, digits...)
}

// allZeroDigits reports whether every digit is "0", the empty string included.
func allZeroDigits(digits string) bool {
	for i := 0; i < len(digits); i++ {
		if digits[i] != '0' {
			return false
		}
	}
	return true
}
