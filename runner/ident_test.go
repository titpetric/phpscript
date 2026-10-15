package runner

import (
	"testing"
)

// TestVarIdent covers the slot spelling of a PHP variable. `$this` keeps the
// bare name and every other variable takes the `v_` prefix, so a variable and
// a bare constant of the same spelling are two slots: `define("x", 1); echo x;`
// reads one, `echo $x;` reads the other.
func TestVarIdent(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		want string
	}{
		{"this", "this"},
		{"x", "v_x"},
		{"rows", "v_rows"},
		{"", "v_"},
	} {
		if got := varIdent(tc.name); got != tc.want {
			t.Errorf("varIdent(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestConstIdent covers the slot spelling of a bare name. The prefix is the
// one resolveVar reads to tell an unresolved constant from a variable nothing
// set, where the variable is null and the constant is a lookup.
func TestConstIdent(t *testing.T) {
	t.Parallel()
	if got, want := constIdent("PHP_EOL"), constIdentPrefix+"PHP_EOL"; got != want {
		t.Errorf("constIdent(%q) = %q, want %q", "PHP_EOL", got, want)
	}
	if constIdent("x") == varIdent("x") {
		t.Error("a constant and a variable of the same spelling share one slot")
	}
}

// TestIsSlotIdent covers which identifiers Eval binds per evaluation: a PHP
// variable, a bare-name constant, a compiled closure literal `__cl<N>`. The
// digit check keeps `__classconst`, which shares the `__cl` prefix, off the
// slot path and on the Base path with the other helpers.
func TestIsSlotIdent(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"this", true},
		{varIdent("rows"), true},
		{constIdent("PHP_EOL"), true},
		{"__cl0", true},
		{"__cl12", true},
		{"__classconst", false},
		{"__cl", false},
		{"concat", false},
	} {
		if got := isSlotIdent(tc.name); got != tc.want {
			t.Errorf("isSlotIdent(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
