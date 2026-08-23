package runner_test

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// TestBitwiseSemantics pins the operand coercions and the shift edges. Every
// expectation is what the php binary prints for the same expression, including
// the ones PHP only reaches after a "non-numeric value" warning.
func TestBitwiseSemantics(t *testing.T) {
	tests := []struct {
		expr string
		want string
	}{
		// Operands are cast to int.
		{`"12abc" & 3`, "0"},
		{`"12abc" | 3`, "15"},
		{`true | 0`, "1"},
		{`false | 0`, "0"},
		{`1.9 & 3`, "1"},
		{`-1.9 | 0`, "-1"},
		{`null | 5`, "5"},
		{`null & 5`, "0"},
		{`"3" & 1`, "1"},

		// Two strings are combined byte by byte and yield a string. `&` and
		// `^` stop at the shorter operand, `|` keeps the longer one.
		{`"3" & "1"`, "1"},
		{`"a" | "b"`, "c"},
		{`"abc" & "ab"`, "ab"},
		{`"abc" | "ab"`, "abc"},
		{`strlen("abc" ^ "ab")`, "2"},
		{`"Hello" ^ "    "`, "hELL"},

		// ~n is -(n+1) on an int and a byte flip on a string.
		{`~5`, "-6"},
		{`~-6`, "5"},
		{`~0`, "-1"},
		{`~1.9`, "-2"},
		{`~ ~"abc"`, "abc"},
		{`strlen(~"abc")`, "3"},

		// Shifts are int64 operations: an over-wide count is 0, or -1 for a
		// negative right operand, and a left shift wraps.
		{`1 << 63`, "-9223372036854775808"},
		{`1 << 64`, "0"},
		{`-1 >> 63`, "-1"},
		{`-1 >> 64`, "-1"},
		{`5 >> 70`, "0"},
		{`PHP_INT_MAX << 1`, "-2"},
		{`-8 >> 1`, "-4"},
	}

	for _, test := range tests {
		t.Run(test.expr, func(t *testing.T) {
			if got := evalPHP(t, "<?php echo "+test.expr+";"); got != test.want {
				t.Errorf("%s = %q, want %q", test.expr, got, test.want)
			}
		})
	}
}

// TestBitwiseNegativeShiftIsCatchable pins the one bitwise input with no answer.
// PHP raises ArithmeticError for it; a compound assignment has to fail the same
// way, or `$x >>= -1` would assign a value the expression form refuses.
func TestBitwiseNegativeShiftIsCatchable(t *testing.T) {
	for _, src := range []string{
		`<?php try { echo 1 << -1; } catch (ArithmeticError $e) { echo get_class($e), ":", $e->getMessage(); }`,
		`<?php $x = 1; try { $x >>= -1; } catch (ArithmeticError $e) { echo get_class($e), ":", $e->getMessage(); }`,
	} {
		want := "ArithmeticError:Bit shift by negative number"
		if got := evalPHP(t, src); got != want {
			t.Errorf("%s\n  = %q, want %q", src, got, want)
		}
	}
}

// evalPHP runs one snippet on the interpreter and returns what it echoed.
func evalPHP(t *testing.T, src string) string {
	t.Helper()
	prog, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out strings.Builder
	rt := runner.New(&out, runner.Options{})
	stdlib.Register(rt)
	if err := rt.Run(prog); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out.String()
}
