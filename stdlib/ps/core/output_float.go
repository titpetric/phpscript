package core

import (
	"math"
	"strconv"
	"strings"
)

// phpFormatFloat renders f with precision significant digits, a negative precision naming the shortest round-trip form.
func phpFormatFloat(f float64, precision int) string {
	switch {
	case math.IsInf(f, 1):
		return "INF"
	case math.IsInf(f, -1):
		return "-INF"
	case math.IsNaN(f):
		return "NAN"
	}

	ndigit, digits := precision, precision-1
	if precision < 0 {
		// The shortest round-tripping form never needs more than 17 digits,
		// and 17 is the width PHP compares the decimal point against.
		ndigit, digits = 17, -1
	}
	// 'e' puts the significant digits and the decimal exponent in a fixed
	// place, which the two decisions below read; 'g' would already
	// have made them, differently.
	s := strconv.FormatFloat(f, 'e', digits, 64)
	sign := ""
	if s[0] == '-' {
		sign, s = "-", s[1:]
	}
	mark := strings.IndexByte(s, 'e')
	exp, err := strconv.Atoi(s[mark+1:])
	if err != nil {
		return s
	}
	mantissa := strings.TrimRight(strings.Replace(s[:mark], ".", "", 1), "0")
	if mantissa == "" {
		mantissa = "0"
	}
	// point is the position of the decimal point relative to the digits:
	// 0.<digits> * 10^point, PHP's own convention for the comparison below.
	point := exp + 1

	if point < -3 || point > ndigit {
		fraction := mantissa[1:]
		if fraction == "" {
			fraction = "0"
		}
		return sign + mantissa[:1] + "." + fraction + "E" + expSign(exp) + strconv.Itoa(abs(exp))
	}
	switch {
	case point <= 0:
		return sign + "0." + strings.Repeat("0", -point) + mantissa
	case point >= len(mantissa):
		return sign + mantissa + strings.Repeat("0", point-len(mantissa))
	default:
		return sign + mantissa[:point] + "." + mantissa[point:]
	}
}

func expSign(exp int) string {
	if exp < 0 {
		return "-"
	}
	return "+"
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
