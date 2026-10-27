package lint

import (
	"fmt"

	"github.com/titpetric/phpscript/model"
)

// lintReferences reports the reference markers that parse and confer nothing
// (docs/design.md, "`&` outside `foreach`"): `$a = &$b` binds by value, and
// `function &f()` returns by value. Both spellings survive the formatter and
// the linter is where a port hears that the aliasing they promise never
// happens. The `&` of `foreach ($a as &$v)`, a parameter or a closure `use`
// never builds these nodes and is not reported.
func lintReferences(file string, prog *model.Program, out *[]Diagnostic) {
	w := &astWalker{prog: prog}
	byRefDecl := func(fallback model.Stmt, fn *model.FuncDecl, name string) {
		if !fn.ByRef {
			return
		}
		line := prog.SourceSpans[fallback].Start
		if span, ok := prog.SourceSpans[fn]; ok {
			line = span.Start
		}
		*out = append(*out, Diagnostic{
			File:    file,
			Line:    line,
			Message: fmt.Sprintf("function &%s() returns by value: the & is a no-op; return the value (docs/design.md)", name),
		})
	}
	w.stmt = func(s model.Stmt) {
		switch n := s.(type) {
		case *model.FuncDecl:
			byRefDecl(n, n, n.Name)
		case *model.ClassDecl:
			for _, m := range n.Methods {
				byRefDecl(n, m, n.Name+"::"+m.Name)
			}
		}
	}
	w.expr = func(e model.Expr, line int) {
		switch n := e.(type) {
		case *model.Ref:
			*out = append(*out, Diagnostic{
				File:    file,
				Line:    line,
				Message: "reference & is a no-op: the value is bound by value, and a later write through one name is not seen through the other (docs/design.md)",
			})
		case *model.Closure:
			if n.ByRef {
				*out = append(*out, Diagnostic{
					File:    file,
					Line:    line,
					Message: "function &() returns by value: the & is a no-op; return the value (docs/design.md)",
				})
			}
		}
	}
	w.walk(prog.Stmts)
}

// lintChainedAssign reports `$a = $b = value`, a single statement binding one
// value to two or more names.
//
// PHP copies an array on assignment and there each name holds its own
// array. phpscript's arrays are references and one array sits
// under both names and a later write through either is visible through the
// other, a bug
// the shape hides and never announces.
//
// The rule sees only the chains the parser left after fixing the ones it can.
// The parser splits a chain ending in an array literal into one allocation
// per name (parser.lowerChainedAlloc), and that chain never reaches here; it
// leaves a chain ending in a scalar literal alone, and this check skips it,
// because a string or a number has no interior for the names to share. What
// remains is a chain ending in a name, a call or a `new`, where either the
// value is a handle the names really do share -- `$dba = $dbb = new Database`
// leaves one connection under two names in PHP as well -- or its type is not
// known until the statement runs. Both are worth reading again, and the
// finding says so.
func lintChainedAssign(file string, n *model.Assign, out *[]Diagnostic) {
	chained, ok := model.UnwrapParenthesized(n.Value).(*model.AssignExpr)
	if !ok {
		return
	}
	if isScalarLiteral(chainedValue(chained)) {
		return
	}
	*out = append(*out, Diagnostic{
		File:    file,
		Line:    chained.Line,
		Message: "chained assignment binds one value to several names",
	})
}

// chainedValue walks to the end of an assignment chain and the check judges
// `$a = $b = $c = '00'` by the `'00'` and not by the assignment that binds
// `$c`.
func chainedValue(n *model.AssignExpr) model.Expr {
	v := model.UnwrapParenthesized(n.Value)
	for {
		next, ok := v.(*model.AssignExpr)
		if !ok {
			return v
		}
		v = model.UnwrapParenthesized(next.Value)
	}
}

// isScalarLiteral reports whether the source spells out a value that no name
// can share: a string, int, float, bool or null literal, an interpolated
// string, which always evaluates to a string, or a prefix operator over one of
// those and a negative number counts. A *model.Lit holding a *model.Array is
// not one of these; the parser does not build them, but the check reads the
// value and never guesses it.
func isScalarLiteral(e model.Expr) bool {
	switch v := e.(type) {
	case *model.Interp:
		return true
	case *model.Lit:
		switch v.Value.(type) {
		case string, int, int64, float64, bool, nil:
			return true
		}
	case *model.Unary:
		switch v.Op {
		case "-", "+", "!", "~":
			return !v.Postfix && isScalarLiteral(model.UnwrapParenthesized(v.X))
		}
	}
	return false
}

func lintCondition(file string, e model.Expr, out *[]Diagnostic) {
	for _, a := range assignExprs(e) {
		*out = append(*out, Diagnostic{
			File:    file,
			Line:    a.Line,
			Message: "assignment in conditional statement",
		})
	}
}

func assignExprs(e model.Expr) []*model.AssignExpr {
	var out []*model.AssignExpr
	collectAssignExprs(e, &out)
	return out
}

func collectAssignExprs(e model.Expr, out *[]*model.AssignExpr) {
	switch n := e.(type) {
	case nil:
	case *model.AssignExpr:
		*out = append(*out, n)
		collectAssignExprs(n.Value, out)
	case *model.Unary:
		collectAssignExprs(n.X, out)
	case *model.Ref:
		collectAssignExprs(n.X, out)
	case *model.Parenthesized:
		collectAssignExprs(n.X, out)
	case *model.Binary:
		collectAssignExprs(n.Left, out)
		collectAssignExprs(n.Right, out)
	case *model.Interp:
		collectAssignExprList(n.Parts, out)
	case *model.Ternary:
		collectAssignExprs(n.Cond, out)
		collectAssignExprs(n.Then, out)
		collectAssignExprs(n.Else, out)
	case *model.Cast:
		collectAssignExprs(n.X, out)
	case *model.Index:
		collectAssignExprs(n.Base, out)
		collectAssignExprs(n.Index, out)
	case *model.PropAccess:
		collectAssignExprs(n.Base, out)
	case *model.MethodCall:
		collectAssignExprs(n.Base, out)
		collectAssignExprList(n.Args, out)
	case *model.Call:
		collectAssignExprList(n.Args, out)
	case *model.New:
		collectAssignExprList(n.Args, out)
	case *model.ArrayLit:
		for _, it := range n.Items {
			collectAssignExprs(it.Key, out)
			collectAssignExprs(it.Val, out)
		}
	case *model.ListExpr:
		collectAssignExprList(n.Elems, out)
	}
}

func collectAssignExprList(xs []model.Expr, out *[]*model.AssignExpr) {
	for _, x := range xs {
		collectAssignExprs(x, out)
	}
}
