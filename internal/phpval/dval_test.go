package phpval_test

import (
	"math"
	"testing"

	"github.com/titpetric/phpscript/internal/phpval"
)

// TestToInt64 covers what a .phpt cannot reach: NAN and INF are not registered
// constants, and the exact range boundaries are easier to state here than to
// spell as PHP literals. Every expected value is what php 8.5 prints for
// (int) of the same double.
func TestToInt64(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want int64
	}{
		{"zero", 0, 0},
		{"truncates toward zero", 1.9, 1},
		{"truncates toward zero, negative", -1.9, -1},
		{"largest that fits", 9.0e18, 9000000000000000000},
		{"2^63 is the first that does not fit", 9223372036854775808.0, math.MinInt64},
		{"2^64 wraps to zero", 18446744073709551616.0, 0},
		{"1e19 wraps negative", 1.0e19, -8446744073709551616},
		{"1e20 wraps positive", 1.0e20, 7766279631452241920},
		{"-1e20 wraps negative", -1.0e20, -7766279631452241920},
		{"2^64 + 2^63", 27670116110564327424.0, math.MinInt64},
		{"NaN is zero", math.NaN(), 0},
		{"positive infinity is zero", math.Inf(1), 0},
		{"negative infinity is zero", math.Inf(-1), 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := phpval.ToInt64(tc.in); got != tc.want {
				t.Fatalf("ToInt64(%v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
