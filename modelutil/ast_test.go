package modelutil_test

import (
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// TestUnwrapParenthesized covers reading an expression's shape without
// evaluating it. A consumer that switches on the node type needs grouping to be
// transparent and `($a)` and `((($a)))` both read as the variable.
//
// The grouping stays on the tree. The formatter rewrites files in place and
// prints the parentheses an author wrote and peeling them copies nothing and
// edits nothing.
func TestUnwrapParenthesized(t *testing.T) {
	t.Parallel()
	v := &model.Var{Name: "v"}
	one := &model.Lit{Value: int64(1)}

	for _, tc := range []struct {
		name string
		expr model.Expr
		want model.Expr
	}{
		{"bare", v, v},
		{"one layer", &model.Parenthesized{X: v}, v},
		{"three layers", &model.Parenthesized{X: &model.Parenthesized{X: &model.Parenthesized{X: v}}}, v},
		{"literal", one, one},
		{"nothing", nil, nil},
	} {
		if got := modelutil.UnwrapParenthesized(tc.expr); got != tc.want {
			t.Errorf("%s: UnwrapParenthesized() = %#v, want %#v", tc.name, got, tc.want)
		}
	}

	grouped := &model.Binary{
		Op:    "*",
		Left:  &model.Parenthesized{X: &model.Binary{Op: "+", Left: v, Right: one}},
		Right: one,
	}
	if modelutil.UnwrapParenthesized(grouped.Left) == grouped.Left {
		t.Fatal("UnwrapParenthesized answered the grouping it was given")
	}
	if _, ok := grouped.Left.(*model.Parenthesized); !ok {
		t.Error("the grouping came off the tree, so a printer cannot put it back")
	}
}
