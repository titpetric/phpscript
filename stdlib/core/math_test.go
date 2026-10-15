package core_test

import (
	"testing"
)

// TestHexdec covers the reading hexdec() makes: hex digits until the first
// character that is not one, with the 0x prefix accepted and a string holding
// no hex digit at all answering 0 and raising nothing.
func TestHexdec(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(hexdec("1A"), hexdec("0x1A"), hexdec("ff"), hexdec("zz"), hexdec(""), hexdec("10"));`)

	want := "int(26)\nint(26)\nint(255)\nint(0)\nint(0)\nint(16)\n"
	if got != want {
		t.Errorf("hexdec = %q, want %q", got, want)
	}
}

// TestAbs covers the type abs() answers in, which is the argument's: an int
// stays an int and a float stays a float, and a numeric string is read as the
// number it spells, so abs("-7") is the int 7.
func TestAbs(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(abs(-5), abs(5), abs(-5.5), abs("-7"), abs(0), abs(-0.0));`)

	want := "int(5)\nint(5)\nfloat(5.5)\nint(7)\nint(0)\nfloat(0)\n"
	if got != want {
		t.Errorf("abs = %q, want %q", got, want)
	}
}

// TestPow covers the type pow() answers in. An integer base raised to a
// non-negative integer exponent stays an int while the product fits, and
// becomes a float where it would overflow, which is php's own promotion: 2**62
// is an int and 2**63 is a float.
//
// A negative or fractional exponent is a float whatever the base, because the
// answer is not a whole number of the base.
func TestPow(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(pow(2, 10), pow(2, 62), pow(2, 63), pow(3, 40), pow(0, 0));
var_dump(pow(2, -1), pow(2.0, 3), pow(2, 0.5));`)

	want := "int(1024)\nint(4611686018427387904)\nfloat(9.223372036854776E+18)\n" +
		"float(1.2157665459056929E+19)\nint(1)\n" +
		"float(0.5)\nfloat(8)\nfloat(1.4142135623730951)\n"
	if got != want {
		t.Errorf("pow = %q, want %q", got, want)
	}
}

// TestRound covers php's rounding, which is half away from zero and not Go's
// half to even: round(2.5) is 3 and round(-2.5) is -3, where Go's math.Round
// agrees and its banker's rounding would not.
//
// A precision argument rounds to that many decimals, and 1.005 is the case
// that catches a naive multiply-and-round: the float nearest 1.005 is below
// it, so a decimal reading is what answers 1.01.
func TestRound(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(round(2.5), round(-2.5), round(3.5), round(2.4), round(-2.4));
var_dump(round(1.2345, 2), round(1.005, 2), round(1.0, 2), round(1234.5678, -2));`)

	want := "float(3)\nfloat(-3)\nfloat(4)\nfloat(2)\nfloat(-2)\n" +
		"float(1.23)\nfloat(1.01)\nfloat(1)\nfloat(1200)\n"
	if got != want {
		t.Errorf("round = %q, want %q", got, want)
	}
}

// TestNumberFormat covers the four arguments and their defaults: no decimals, a
// comma between thousands and a dot before the decimals. The separators are
// taken as given, including an empty thousands separator, and the value is
// rounded and not truncated, so 1234.5678 with no decimals is 1,235.
func TestNumberFormat(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(number_format(1234.5678), number_format(1234.5678, 2));
var_dump(number_format(1234.5678, 2, ",", "."), number_format(1234.5678, 2, ",", ""));
var_dump(number_format(1000000), number_format(-0.5), number_format(0.5), number_format(0));`)

	want := "string(5) \"1,235\"\nstring(8) \"1,234.57\"\n" +
		"string(8) \"1.234,57\"\nstring(7) \"1234,57\"\n" +
		"string(9) \"1,000,000\"\nstring(2) \"-1\"\nstring(1) \"1\"\nstring(1) \"0\"\n"
	if got != want {
		t.Errorf("number_format = %q, want %q", got, want)
	}
}

// TestMinMax covers the two spellings php accepts: several arguments, or one
// array holding them. The comparison is php's, so a mixed list is compared the
// way php compares those values, and the winner keeps its own type.
func TestMinMax(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(min(3, 1, 2), max(3, 1, 2));
var_dump(min([3, 1, 2]), max([3, 1, 2]));
var_dump(min("a", "b"), max(1, "2", 3.5));`)

	want := "int(1)\nint(3)\nint(1)\nint(3)\nstring(1) \"a\"\nfloat(3.5)\n"
	if got != want {
		t.Errorf("min/max = %q, want %q", got, want)
	}
}

// TestLog covers the base argument. With none the base is e, so log(M_E) is 1;
// with one the result is the logarithm in that base, so log(8, 2) is 3 and the
// caller divides nothing.
func TestLog(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(log(M_E), log(8, 2), log(100, 10), log(1));`)

	want := "float(1)\nfloat(3)\nfloat(2)\nfloat(0)\n"
	if got != want {
		t.Errorf("log = %q, want %q", got, want)
	}
}
