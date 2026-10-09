package phpval

import (
	"math"
)

// Bounds of the int64 range as float64. A double at or above 2^63 does not fit,
// and 2^63 is exactly representable, so the test is a half-open interval.
const (
	twoPow63 = 9223372036854775808.0  // 2^63
	twoPow64 = 18446744073709551616.0 // 2^64
)

// ToInt64 converts a float to the integer PHP converts it to.
//
// Go's conversion of an out-of-range float to int64 is implementation defined,
// and on amd64 it saturates: every value too large becomes math.MinInt64. PHP
// wraps instead, modulo 2^64 into the signed range, which is what C's
// (int64_t) cast does on the same hardware and what zend_dval_to_lval
// implements. The two disagree on every value outside the range:
//
//	(int)1.0e20   php 7766279631452241920    Go -9223372036854775808
//	(int)1.0e19   php -8446744073709551616   Go -9223372036854775808
//	(int)2.0**64  php 0                      Go -9223372036854775808
//
// NaN and both infinities are zero, as PHP 8 answers them.
//
// A value already inside the range takes a plain conversion, so the arithmetic
// below is only reached by a float that does not fit. That matters: this is on
// the path every modulo, bitwise operator and integer cast takes.
func ToInt64(f float64) int64 {
	if f >= -twoPow63 && f < twoPow63 {
		return int64(f)
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}

	// Reduce modulo 2^64, then fold the upper half of the unsigned range onto
	// the negative half of the signed one. Both branches are needed: fmod keeps
	// the sign of its argument, so a negative input lands in (-2^64, 0].
	mod := math.Mod(f, twoPow64)
	if mod < 0 {
		mod += twoPow64
	}
	if mod >= twoPow63 {
		mod -= twoPow64
	}
	return int64(mod)
}
