package core_test

import (
	"testing"
)

// TestFloatFormatting covers where php switches a float to the exponent form:
// the decimal point decides, not Go's 'g' thresholds. 1e16 is still written
// out at seventeen digits; 1e17 is not. A small value turns at the third
// leading zero. The mantissa always holds a fraction, the exponent no leading
// zero. The spelling is 1.0E+17 and not 1e+17.
//
// echo passes precision=14 where var_dump passes the round-tripping form; one
// float has two spellings: 1/3 is 0.33333333333333 to echo.
func TestFloatFormatting(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_dump(1e15, 1e16, 1e17, 1e-4, 1e-5, 1.0E+100, -0.0);
echo 1e16, " ", 0.1 + 0.2, " ", 1/3, "\n";`)

	want := "float(1000000000000000)\nfloat(10000000000000000)\nfloat(1.0E+17)\n" +
		"float(0.0001)\nfloat(1.0E-5)\nfloat(1.0E+100)\nfloat(-0)\n" +
		"1.0E+16 0.3 0.33333333333333\n"
	if got != want {
		t.Errorf("float formatting =\n%q\nwant\n%q", got, want)
	}
}

// TestFloatNonFinite covers three values that are not numbers; each prints as
// a name in all renderers, showing no sign in var_export.
func TestFloatNonFinite(t *testing.T) {
	t.Parallel()
	got := runPHP(t, `<?php
var_export(INF);
echo "\n";
var_dump(INF, -INF, NAN);`)

	want := "INF\nfloat(INF)\nfloat(-INF)\nfloat(NAN)\n"
	if got != want {
		t.Errorf("non-finite floats = %q, want %q", got, want)
	}
}
