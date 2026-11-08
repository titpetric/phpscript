package modelutil

import (
	"github.com/titpetric/phpscript/model"
)

// This file holds the two pieces of PHP's value semantics that a by-value
// `foreach` needs: what copying an array means, and whether a loop body would
// notice the difference.
//
// PHP arrays are values. `foreach ($rows as $row)` binds a copy and
// `$row["x"] = 1` edits the copy and leaves `$rows` alone; `foreach ($rows as
// &$row)` binds the element itself. phpscript's arrays are pointers and a
// by-value loop copies up front and never defers: there is no refcount to make
// it lazy the way PHP's copy-on-write does.
//
// Copying every element of every loop would be a large price for a semantic
// almost no loop uses and a loop copies only when the body actually assigns
// through the loop variable. AssignsTo answers that, once per loop and not
// once per iteration, and the flatstack compiler asks it once per program.

// CopyValue returns a value with PHP's assignment semantics applied: a deep
// copy of an array, anything else unchanged. An object is a handle in PHP too.
func CopyValue(v any) any {
	array, ok := v.(*model.Array)
	if !ok {
		return v
	}
	return CopyArray(array)
}

// CopyArray returns an independent copy of a, with nested arrays copied too.
func CopyArray(a *model.Array) *model.Array {
	if a == nil {
		return nil
	}
	out := model.NewArraySize(a.Len())
	a.Range(func(key, val any) bool {
		out.Set(key, CopyValue(val))
		return true
	})
	return out
}

// AssignsTo reports whether any statement in body writes through target's root
// name. A write to `$v["k"]` matches `$v`: it reaches inside the value.
func AssignsTo(target model.Expr, body []model.Stmt) bool {
	root := RootName(target)
	if root == "" {
		// A target with no name to root on matches nothing and treat it as a
		// write the body makes.
		return true
	}
	return stmtsAssignTo(body, root)
}

// RootName returns the variable name at the root of an lvalue: `$v`, `$v["k"]`
// and `$v->p->q` all root at "v". It returns "" for anything else.
func RootName(e model.Expr) string {
	for {
		switch n := UnwrapParenthesized(e).(type) {
		case *model.Var:
			if n.Const {
				return ""
			}
			return n.Name
		case *model.Index:
			e = n.Base
		case *model.PropAccess:
			e = n.Base
		default:
			return ""
		}
	}
}

func stmtsAssignTo(stmts []model.Stmt, root string) bool {
	for _, s := range stmts {
		if stmtAssignsTo(s, root) {
			return true
		}
	}
	return false
}

func stmtAssignsTo(s model.Stmt, root string) bool {
	switch n := s.(type) {
	case *model.Assign:
		return RootName(n.Target) == root || listAssignsTo(n.Target, root) || exprAssignsTo(n.Value, root)
	case *model.ExprStmt:
		return exprAssignsTo(n.X, root)
	case *model.Echo:
		return exprsAssignTo(n.Args, root)
	case *model.If:
		return exprAssignsTo(n.Cond, root) || stmtsAssignTo(n.Then, root) || stmtsAssignTo(n.Else, root)
	case *model.For:
		return stmtAssignsTo(n.Init, root) || exprAssignsTo(n.Cond, root) ||
			stmtAssignsTo(n.Post, root) || stmtsAssignTo(n.Body, root)
	case *model.Foreach:
		// A nested loop that binds the same name rebinds it on every iteration,
		// which the enclosing loop counts as a write.
		return RootName(n.ValTarget) == root || RootName(n.KeyTarget) == root ||
			n.ValVar == root || n.KeyVar == root ||
			exprAssignsTo(n.Source, root) || stmtsAssignTo(n.Body, root)
	case *model.Switch:
		if exprAssignsTo(n.Cond, root) || stmtsAssignTo(n.Default, root) {
			return true
		}
		for _, c := range n.Cases {
			if exprAssignsTo(c.Value, root) || stmtsAssignTo(c.Body, root) {
				return true
			}
		}
	case *model.Try:
		if stmtsAssignTo(n.Body, root) || stmtsAssignTo(n.Finally, root) {
			return true
		}
		for _, c := range n.Catches {
			if c.Var == root || stmtsAssignTo(c.Body, root) {
				return true
			}
		}
	case *model.Return:
		return exprAssignsTo(n.Value, root)
	case *model.Throw:
		return exprAssignsTo(n.X, root)
	case *model.Unset:
		for _, t := range n.Targets {
			if RootName(t) == root {
				return true
			}
		}
	case *model.Include:
		// An included file is not visible here. It shares this scope.
		return true
	}
	return false
}

// listAssignsTo reports whether a list() destructuring target names root.
func listAssignsTo(target model.Expr, root string) bool {
	list, ok := UnwrapParenthesized(target).(*model.ListExpr)
	if !ok {
		return false
	}
	for _, el := range list.Elems {
		if el != nil && RootName(el) == root {
			return true
		}
	}
	return false
}

func exprsAssignTo(xs []model.Expr, root string) bool {
	for _, x := range xs {
		if exprAssignsTo(x, root) {
			return true
		}
	}
	return false
}

func exprAssignsTo(e model.Expr, root string) bool {
	switch n := e.(type) {
	case nil:
		return false
	case *model.AssignExpr:
		return RootName(n.Target) == root || listAssignsTo(n.Target, root) || exprAssignsTo(n.Value, root)
	case *model.Unary:
		// `$v++` and `--$v` write; every other unary only reads.
		if (n.Op == "++" || n.Op == "--") && RootName(n.X) == root {
			return true
		}
		return exprAssignsTo(n.X, root)
	case *model.Parenthesized:
		return exprAssignsTo(n.X, root)
	case *model.Binary:
		return exprAssignsTo(n.Left, root) || exprAssignsTo(n.Right, root)
	case *model.Interp:
		return exprsAssignTo(n.Parts, root)
	case *model.Coalesce:
		return exprAssignsTo(n.Left, root) || exprAssignsTo(n.Right, root)
	case *model.Ternary:
		return exprAssignsTo(n.Cond, root) || exprAssignsTo(n.Then, root) || exprAssignsTo(n.Else, root)
	case *model.Cast:
		return exprAssignsTo(n.X, root)
	case *model.Index:
		return exprAssignsTo(n.Base, root) || exprAssignsTo(n.Index, root)
	case *model.PropAccess:
		return exprAssignsTo(n.Base, root) || exprAssignsTo(n.NameExpr, root)
	case *model.MethodCall:
		return exprAssignsTo(n.Base, root) || exprsAssignTo(n.Args, root)
	case *model.Call:
		// A shim with an output parameter writes back into the frame, and the
		// by-reference table is not visible from here.
		return exprsAssignTo(n.Args, root) || callWritesBack(n, root)
	case *model.StaticCall:
		return exprsAssignTo(n.Args, root)
	case *model.Invoke:
		return exprAssignsTo(n.Callee, root) || exprsAssignTo(n.Args, root)
	case *model.New:
		return exprsAssignTo(n.Args, root)
	case *model.ArrayLit:
		for _, it := range n.Items {
			if exprAssignsTo(it.Key, root) || exprAssignsTo(it.Val, root) {
				return true
			}
		}
	case *model.ListExpr:
		return exprsAssignTo(n.Elems, root)
	case *model.Closure:
		// A closure captures by value and its body cannot write to this frame.
		return false
	case *model.Include:
		return true
	}
	return false
}

// callWritesBack reports whether a call passes root to an output parameter.
func callWritesBack(n *model.Call, root string) bool {
	for _, arg := range n.Args {
		if v, ok := UnwrapParenthesized(arg).(*model.Var); ok && !v.Const && v.Name == root {
			return true
		}
	}
	return false
}
