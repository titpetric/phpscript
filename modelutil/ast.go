package modelutil

import (
	"github.com/titpetric/phpscript/model"
)

// UnwrapParenthesized returns the expression inside any explicit grouping.
// A consumer that inspects expression shape without evaluating it uses
// this so parentheses remain semantically transparent.
func UnwrapParenthesized(e model.Expr) model.Expr {
	for {
		p, ok := e.(*model.Parenthesized)
		if !ok {
			return e
		}
		e = p.X
	}
}
