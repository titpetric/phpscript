package phpval

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

// want below is the value `$x <=> $y` printed for the same pair.
func TestCompare(t *testing.T) {
	list := func(vals ...any) *model.Array {
		a := model.NewArray()
		for _, v := range vals {
			a.Append(v)
		}
		return a
	}
	assoc := func(key string, val any) *model.Array {
		a := model.NewArray()
		a.Set(key, val)
		return a
	}

	cases := []struct {
		x, y any
		want int
	}{
		// numbers
		{10, 9, 1},
		{10, 10, 0},
		{9, 10, -1},
		{10, 9.5, 1},
		{1, 1.0, 0},
		{2.0, "2", 0},

		// a numeric string is a number, which is what sort(explode(...)) needs
		{"10", "9", 1},
		{"10", 9, 1},
		{10, "9", 1},
		{"1e2", "100", 0},
		{" 12 ", "12", 0},
		{"1.", "1", 0},
		{".5", "0.5", 0},

		// PHP's numeric strings stop short of Go's: no hex, no digit
		// separators, no "INF"
		{"0x1A", "26", -1},
		{"1_000", "1000", 1},
		{"INF", "0", 1},
		{"e5", "5e", 1},

		// strings
		{"abc", 10, 1},
		{0, "abc", -1},
		{"abc", "abd", -1},
		{"ABC", "abc", -1},

		// integers past float64's precision keep their digits
		{"9223372036854775807", "9223372036854775806", 1},
		{"9223372036854775808", "9223372036854775807", 1},
		{"18446744073709551616", "18446744073709551617", -1},
		{"9223372036854775808", int64(9223372036854775807), 0},
		{"1e400", "2e400", -1},
		{"1e20", "100000000000000000000", 0},

		// null against a string is "" against that string
		{nil, nil, 0},
		{nil, "a", -1},
		{nil, "", 0},
		{nil, "0", -1},
		{"", nil, 0},

		// null and bool against anything else is truthiness
		{nil, false, 0},
		{nil, 0, 0},
		{nil, 1, -1},
		{true, "abc", 0},
		{false, "0", 0},
		{true, false, 1},
		{true, 1, 0},
		{false, "", 0},
		{true, "0", 1},

		// arrays
		{list(1, 2), list(1, 3), -1},
		{list(1), list(1, 2), -1},
		{list(1, 2), list(1, 2), 0},
		{assoc("a", 1), assoc("b", 1), 1},
		{list(1), 5, 1},
		{5, list(1), -1},
		{model.NewArray(), "", 1},
	}
	for _, c := range cases {
		if got := Compare(c.x, c.y); got != c.want {
			t.Errorf("Compare(%#v, %#v) = %d, want %d", c.x, c.y, got, c.want)
		}
	}
}
