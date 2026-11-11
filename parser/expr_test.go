package parser

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// shape renders an expression fully parenthesized and a test states the tree
// the precedence table built and never the value an engine would compute from
// it. The parser cannot reach the runner and it runs nothing.
func shape(e model.Expr) string {
	switch n := e.(type) {
	case *model.Binary:
		return "(" + shape(n.Left) + " " + n.Op + " " + shape(n.Right) + ")"
	case *model.Unary:
		if n.Postfix {
			return "(" + shape(n.X) + n.Op + ")"
		}
		return "(" + n.Op + shape(n.X) + ")"
	case *model.Coalesce:
		return "(" + shape(n.Left) + " ?? " + shape(n.Right) + ")"
	case *model.Ternary:
		return "(" + shape(n.Cond) + " ? " + shape(n.Then) + " : " + shape(n.Else) + ")"
	case *model.Parenthesized:
		return shape(n.X)
	case *model.AssignExpr:
		return "(" + shape(n.Target) + " " + n.Op + " " + shape(n.Value) + ")"
	case *model.Var:
		if n.Const {
			return n.Name
		}
		return "$" + n.Name
	case *model.Lit:
		if text, ok := n.Value.(string); ok {
			return strconv.Quote(text)
		}
		return fmt.Sprintf("%v", n.Value)
	}
	return fmt.Sprintf("%T", e)
}

// expression parses src as one echo argument and returns its tree.
func expression(t *testing.T, src string) model.Expr {
	t.Helper()
	program := mustParse(t, "<?php echo "+src+";")
	echo, ok := program.Stmts[0].(*model.Echo)
	if !ok || len(echo.Args) != 1 {
		t.Fatalf("%q did not parse as one echo argument: %#v", src, program.Stmts[0])
	}
	return echo.Args[0]
}

// TestBinaryPrecedence covers where each level of the table sits, against what
// php computes for the same source. The bitwise operators are looser than any
// comparison, the shifts are tighter than concatenation and looser than
// addition, and every level binds to the left.
func TestBinaryPrecedence(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		src  string
		want string
	}{
		// php: 1 | 2 == 2 is 1 and the comparison folds first.
		{src: `1 | 2 == 2`, want: `(1 | (2 == 2))`},
		// php: 1 << 2 + 3 is 32 and the addition folds first.
		{src: `1 << 2 + 3`, want: `(1 << (2 + 3))`},
		// php: 1 . 2 << 1 is "14" and the shift folds before the dot.
		{src: `1 . 2 << 1`, want: `(1 . (2 << 1))`},
		// php: 2 + 3 . "x" is "5x" and the addition folds before the dot.
		{src: `2 + 3 . "x"`, want: `((2 + 3) . "x")`},
		{src: `1 + 2 * 3`, want: `(1 + (2 * 3))`},
		{src: `7 % 4 * 2`, want: `((7 % 4) * 2)`},
		{src: `10 - 2 - 3`, want: `((10 - 2) - 3)`},
		{src: `1 < 2 == true`, want: `((1 < 2) == true)`},
		{src: `true || false && false`, want: `(true || (false && false))`},
		{src: `1 & 2 ^ 3 | 4`, want: `(((1 & 2) ^ 3) | 4)`},
		{src: `$a === $b !== $c`, want: `(($a === $b) !== $c)`},
	} {
		if got := shape(expression(t, test.src)); got != test.want {
			t.Errorf("%s parsed as %s, want %s", test.src, got, test.want)
		}
	}
}

// TestCoalesceAndWordOperatorPrecedence covers where `??`, `<=>` and the word
// operators sit, against what php computes for the same source: `??` is right
// associative, looser than `||` and tighter than the ternary, `<=>` shares the
// equality level, and `and`, `xor` and `or` fold in that order below
// assignment.
func TestCoalesceAndWordOperatorPrecedence(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		src  string
		want string
	}{
		{src: `$a ?? $b ?? $c`, want: `($a ?? ($b ?? $c))`},
		{src: `$a || $b ?? $c`, want: `(($a || $b) ?? $c)`},
		{src: `$a ?? $b ? 1 : 2`, want: `(($a ?? $b) ? 1 : 2)`},
		{src: `$a ?? $b = 1`, want: `($a ?? ($b = 1))`},
		{src: `$a ?? $b ??= 1`, want: `($a ?? ($b = ($b ?? 1)))`},
		{src: `true && $x ??= 5`, want: `(true && ($x = ($x ?? 5)))`},
		{src: `1 + 1 <=> 2`, want: `((1 + 1) <=> 2)`},
		{src: `1 <=> 2 + 3`, want: `(1 <=> (2 + 3))`},
		{src: `$a = 1 and 0`, want: `(($a = 1) and 0)`},
		{src: `$a or $b and $c`, want: `($a or ($b and $c))`},
		{src: `$a xor $b or $c`, want: `(($a xor $b) or $c)`},
		{src: `$a or $b xor $c and $d`, want: `($a or ($b xor ($c and $d)))`},
		{src: `$a AND $b`, want: `($a and $b)`},
	} {
		if got := shape(expression(t, test.src)); got != test.want {
			t.Errorf("%s parsed as %s, want %s", test.src, got, test.want)
		}
	}
}

// TestCoalesceAssignLowersToAssignment covers `??=`: an assignment whose value
// is a Coalesce marked Assign, which leaves the runtime one operator fewer. A
// target holding a call takes the same shape; both engines resolve it once.
// The parser refuses an append slot, a target php cannot read either.
func TestCoalesceAssignLowersToAssignment(t *testing.T) {
	t.Parallel()
	if _, err := Parse("<?php $a[] ??= 5;"); err == nil {
		t.Error("an append target of ??= parsed; php cannot read one")
	}
	for _, src := range []string{`<?php $a['k'] ??= 5;`, `<?php $a[k()] ??= 5;`, `<?php f()->p ??= 5;`} {
		program := mustParse(t, src)
		assign, ok := program.Stmts[0].(*model.Assign)
		if !ok || assign.Op != "=" {
			t.Fatalf("%s: got %#v, want an assignment with op =", src, program.Stmts[0])
		}
		co, ok := assign.Value.(*model.Coalesce)
		if !ok || !co.Assign {
			t.Fatalf("%s: value is %#v, want a Coalesce marked Assign", src, assign.Value)
		}
	}
}

// TestParenthesesOverridePrecedence covers the escape from the table: a
// parenthesized group folds first whatever the levels say. The parser keeps
// the node so the formatter prints the parentheses the author wrote.
func TestParenthesesOverridePrecedence(t *testing.T) {
	t.Parallel()
	e := expression(t, `(1 + 2) * 3`)
	if got, want := shape(e), `((1 + 2) * 3)`; got != want {
		t.Errorf("parsed as %s, want %s", got, want)
	}

	binary, ok := e.(*model.Binary)
	if !ok {
		t.Fatalf("got %T, want *model.Binary", e)
	}
	if _, ok := binary.Left.(*model.Parenthesized); !ok {
		t.Errorf("the left side is %T, want the group kept for the formatter", binary.Left)
	}
}

// TestTernaryIsLooserThanEveryBinary covers where the conditional sits: the
// whole comparison is the condition and `$a > 1 ? "y" : "n"` reads one way
// only.
func TestTernaryIsLooserThanEveryBinary(t *testing.T) {
	t.Parallel()
	if got, want := shape(expression(t, `$a > 1 ? "y" : "n"`)), `(($a > 1) ? "y" : "n")`; got != want {
		t.Errorf("parsed as %s, want %s", got, want)
	}
}

// TestAssignmentIsRightAssociative covers the one expression-level assignment
// this parser accepts, the `($x = f()) !== false` idiom. It binds to the right
// and only where an assignment may target the left side and `1 = $a` parses as
// the literal and never as an assignment.
func TestAssignmentIsRightAssociative(t *testing.T) {
	t.Parallel()
	if got, want := shape(expression(t, `$a = $b = 1`)), `($a = ($b = 1))`; got != want {
		t.Errorf("parsed as %s, want %s", got, want)
	}

	// A literal is not an lvalue and nothing consumes the operator and the
	// expression ends at the literal.
	program := mustParse(t, `<?php echo 1;`)
	if _, ok := program.Stmts[0].(*model.Echo).Args[0].(*model.AssignExpr); ok {
		t.Error("a literal parsed as an assignment target")
	}
}

// TestIsLValue covers what may appear on the left of an expression-level
// assignment: a variable, an index, a property, a static property and list(),
// and nothing computed. A call result has nowhere to store a value and
// accepting one would build a tree the runner cannot execute.
func TestIsLValue(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		src  string
		want bool
	}{
		{src: `<?php $a = 1;`, want: true},
		{src: `<?php $a[0] = 1;`, want: true},
		{src: `<?php $a->b = 1;`, want: true},
		{src: `<?php A::$b = 1;`, want: true},
	} {
		program := mustParse(t, test.src)
		assign, ok := program.Stmts[0].(*model.Assign)
		if !ok {
			t.Fatalf("%s parsed as %T, want *model.Assign", test.src, program.Stmts[0])
		}
		if got := isLValue(assign.Target); got != test.want {
			t.Errorf("isLValue(%s) = %t, want %t", shape(assign.Target), got, test.want)
		}
	}

	for _, e := range []model.Expr{
		&model.Lit{Value: int64(1)},
		&model.Call{Name: "f"},
		&model.Binary{Op: "+", Left: &model.Lit{Value: int64(1)}, Right: &model.Lit{Value: int64(2)}},
	} {
		if isLValue(e) {
			t.Errorf("isLValue(%T) = true, want false", e)
		}
	}
}
