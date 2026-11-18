package core

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
)

// formatError reports a rejected format under its PHP class.
type formatError struct {
	class   string
	message string
}

// Error renders the message php reports for the same format.
func (e *formatError) Error() string { return e.message }

// ThrowableClass names the PHP class, implementing runner.Throwable.
func (e *formatError) ThrowableClass() string { return e.class }

// formatSpec holds one conversion read from a format string in PHP's grammar.
type formatSpec struct {
	argnum    int // 1-based; argNext for "the argument after the last one taken"
	width     int
	precision int
	pad       byte
	verb      byte
	left      bool
	sign      bool
	zero      bool
	hasPrec   bool
}

// argNext is argnum's value for a conversion naming no argument.
const argNext = -1

// phpSprintf formats args with PHP's conversions, counting width and precision in bytes.
func phpSprintf(format string, args ...any) (string, error) {
	var out strings.Builder
	out.Grow(len(format) + 16)
	taken := 0
	for i := 0; i < len(format); {
		if format[i] != '%' {
			out.WriteByte(format[i])
			i++
			continue
		}
		i++
		if i < len(format) && format[i] == '%' {
			out.WriteByte('%')
			i++
			continue
		}
		spec, next, err := parseFormatSpec(format, i)
		if err != nil {
			return "", err
		}
		i = next
		// php reads the argument before it looks at the specifier. A format
		// that names an argument the call omits reports the count first.
		arg, err := formatArg(spec, args, &taken)
		if err != nil {
			return "", err
		}
		if spec.verb == 0 {
			return "", &formatError{class: "ValueError", message: "Missing format specifier at end of string"}
		}
		if spec.verb == '%' {
			out.WriteByte('%')
			continue
		}
		part, err := spec.render(arg)
		if err != nil {
			return "", err
		}
		out.WriteString(part)
	}
	return out.String(), nil
}

// parseFormatSpec reads the conversion at i and the index past it, verb zero at format end.
func parseFormatSpec(format string, i int) (formatSpec, int, error) {
	spec := formatSpec{argnum: argNext, pad: ' '}
	// An argument number is digits followed by '$'. Digits not followed by one
	// are the width and the scan rewinds and commits nothing.
	j := i
	for j < len(format) && format[j] >= '0' && format[j] <= '9' {
		j++
	}
	if j > i && j < len(format) && format[j] == '$' {
		n, err := strconv.Atoi(format[i:j])
		if err != nil || n < 1 {
			return spec, j, &formatError{
				class:   "ValueError",
				message: "Argument number specifier must be greater than zero and less than 2147483647",
			}
		}
		spec.argnum = n
		i = j + 1
	}
	for flags := true; flags && i < len(format); {
		switch format[i] {
		case '-':
			spec.left = true
		case '+':
			spec.sign = true
		case ' ':
			spec.pad, spec.zero = ' ', false
		case '0':
			spec.pad, spec.zero = '0', true
		case '\'':
			if i+1 >= len(format) {
				return spec, len(format), &formatError{
					class:   "ValueError",
					message: "Missing padding character",
				}
			}
			i++
			spec.pad, spec.zero = format[i], false
		default:
			flags = false
		}
		if flags {
			i++
		}
	}
	for i < len(format) && format[i] >= '0' && format[i] <= '9' {
		spec.width = spec.width*10 + int(format[i]-'0')
		i++
	}
	if i < len(format) && format[i] == '.' {
		i++
		spec.hasPrec = true
		for i < len(format) && format[i] >= '0' && format[i] <= '9' {
			spec.precision = spec.precision*10 + int(format[i]-'0')
			i++
		}
	}
	if i < len(format) {
		spec.verb = format[i]
		i++
	}
	return spec, i, nil
}

// formatArg returns the named argument, advancing the implicit counter only when unnamed.
func formatArg(spec formatSpec, args []any, taken *int) (any, error) {
	n := spec.argnum
	if n == argNext {
		*taken++
		n = *taken
	}
	if n > len(args) {
		// php counts the format string itself as an argument in this message.
		return nil, &formatError{
			class:   "ArgumentCountError",
			message: fmt.Sprintf("%d arguments are required, %d given", n+1, len(args)+1),
		}
	}
	return args[n-1], nil
}

// render converts one argument under this conversion, padded to width.
func (spec formatSpec) render(arg any) (string, error) {
	switch spec.verb {
	case 's':
		s, err := phpval.StringContext(arg)
		if err != nil {
			return "", err
		}
		if spec.hasPrec && spec.precision < len(s) {
			s = s[:spec.precision]
		}
		return spec.pack(s, false), nil
	case 'd':
		s, negative := phpval.Digits(arg)
		if spec.sign && !negative {
			s = "+" + s
		}
		return spec.pack(s, true), nil
	case 'u':
		return spec.pack(strconv.FormatUint(phpval.Uint(arg), 10), true), nil
	case 'b':
		return spec.pack(strconv.FormatUint(phpval.Uint(arg), 2), true), nil
	case 'o':
		return spec.pack(strconv.FormatUint(phpval.Uint(arg), 8), true), nil
	case 'x':
		return spec.pack(strconv.FormatUint(phpval.Uint(arg), 16), true), nil
	case 'X':
		s := strconv.FormatUint(phpval.Uint(arg), 16)
		return spec.pack(strings.ToUpper(s), true), nil
	case 'c':
		// A character conversion takes neither width nor precision in php.
		return string([]byte{byte(phpval.Int(arg))}), nil
	case 'e', 'E', 'f', 'F', 'g', 'G':
		return spec.pack(spec.float(phpval.Float(arg)), true), nil
	}
	return "", &formatError{
		class:   "ValueError",
		message: fmt.Sprintf("Unknown format specifier %q", string(spec.verb)),
	}
}

// float renders a conversion, spelling INF, -INF and NaN and trimming the exponent.
func (spec formatSpec) float(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "INF"
	case math.IsInf(f, -1):
		return "-INF"
	case f == 0:
		f = 0
	}
	prec := 6
	if spec.hasPrec {
		prec = spec.precision
	}
	verb := spec.verb
	switch verb {
	case 'F':
		verb = 'f'
	case 'g', 'G':
		// C and php read a precision of zero as a single significant digit; Go
		// reads it as "the smallest number of digits necessary".
		if prec == 0 {
			prec = 1
		}
	}
	s := strconv.FormatFloat(f, byte(verb), prec, 64)
	if spec.sign && f >= 0 {
		s = "+" + s
	}
	return trimExponent(s)
}

// trimExponent drops leading zeros from an exponent; e+03 reads back as e+3.
func trimExponent(s string) string {
	i := strings.IndexAny(s, "eE")
	if i < 0 {
		return s
	}
	mantissa, exponent := s[:i+1], s[i+1:]
	sign := ""
	if exponent != "" && (exponent[0] == '+' || exponent[0] == '-') {
		sign, exponent = exponent[:1], exponent[1:]
	}
	if trimmed := strings.TrimLeft(exponent, "0"); trimmed != "" {
		exponent = trimmed
	} else if exponent != "" {
		exponent = "0"
	}
	return mantissa + sign + exponent
}

// pack pads a conversion to width, placing zero padding after a numeric sign as PHP does.
func (spec formatSpec) pack(s string, numeric bool) string {
	if spec.width <= len(s) {
		return s
	}
	n := spec.width - len(s)
	if spec.left {
		pad := spec.pad
		if spec.zero {
			pad = ' '
		}
		return s + strings.Repeat(string(pad), n)
	}
	if spec.zero && numeric && s != "" && (s[0] == '-' || s[0] == '+') {
		return s[:1] + strings.Repeat("0", n) + s[1:]
	}
	return strings.Repeat(string(spec.pad), n) + s
}
