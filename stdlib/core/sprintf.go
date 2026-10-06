package core

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/titpetric/phpscript/internal/phpval"
)

// formatError reports a format string php rejects, under the class php raises
// for it: ValueError for a specifier it cannot read, ArgumentCountError for an
// argument the format asks for and the call did not pass.
type formatError struct {
	class   string
	message string
}

// Error renders the message php carries for the same format.
func (e *formatError) Error() string { return e.message }

// ThrowableClass names the PHP class, implementing runner.Throwable.
func (e *formatError) ThrowableClass() string { return e.class }

// formatSpec is one conversion read off a format string, in php's grammar:
// %[argnum$][flags][width][.precision]specifier.
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

// argNext is argnum's value for a conversion that did not name an argument.
const argNext = -1

// phpSprintf formats args into format the way php's sprintf does.
//
// The conversion is php's, not Go's: an argument is coerced by phpval before it
// is rendered, so %s of 42 is "42" rather than fmt's %!s(int64=42), and width,
// precision and padding count bytes, which is the unit the str* functions use.
// docs/reference/extensions/strings.md owns the table of specifiers.
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
		// php reads the argument before it looks at the specifier, so a format
		// that asks for an argument it was not given reports the count first.
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

// parseFormatSpec reads one conversion starting at i, the byte after the '%',
// and returns it with the index of the first byte past it. A spec whose verb is
// zero ran off the end of the format, which the caller reports.
func parseFormatSpec(format string, i int) (formatSpec, int, error) {
	spec := formatSpec{argnum: argNext, pad: ' '}
	// An argument number is digits followed by '$'. Digits not followed by one
	// are the width, so the scan rewinds rather than committing.
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

// formatArg answers the argument a conversion names, advancing the implicit
// counter only for a conversion that did not name one: php's own counter works
// the same way, so `%1$s %s` reads the first argument twice.
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
		s := phpval.String(arg)
		if spec.hasPrec && spec.precision < len(s) {
			s = s[:spec.precision]
		}
		return spec.pack(s, false), nil
	case 'd':
		n := phpval.Int(arg)
		s := strconv.FormatInt(n, 10)
		if spec.sign && n >= 0 {
			s = "+" + s
		}
		return spec.pack(s, true), nil
	case 'u':
		return spec.pack(strconv.FormatUint(uint64(phpval.Int(arg)), 10), true), nil
	case 'b':
		return spec.pack(strconv.FormatUint(uint64(phpval.Int(arg)), 2), true), nil
	case 'o':
		return spec.pack(strconv.FormatUint(uint64(phpval.Int(arg)), 8), true), nil
	case 'x':
		return spec.pack(strconv.FormatUint(uint64(phpval.Int(arg)), 16), true), nil
	case 'X':
		s := strconv.FormatUint(uint64(phpval.Int(arg)), 16)
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

// float renders a floating conversion. php spells a non-finite value as INF,
// -INF or NaN whichever conversion asked for it, prints a negative zero
// unsigned, and writes an exponent with the digits it needs rather than padding
// it to two the way C and Go do.
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
		// C and php read a precision of zero as one significant digit; Go
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

// trimExponent drops the leading zeros php does not write in an exponent, so
// Go's 1.234500e+03 reads back as php's 1.234500e+3.
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

// pack pads a rendered conversion to the spec's width.
//
// Zero padding goes after the sign of a numeric conversion, which is what makes
// %05d of -42 read -0042; a padding character the format named goes before it,
// and a left-aligned conversion pads with spaces where the zero flag asked for
// zeros, both as php does.
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
