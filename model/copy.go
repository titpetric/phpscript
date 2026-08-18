package model

// This file holds the two pieces of PHP's value semantics that a by-value
// `foreach` needs: what copying an array means, and whether a loop body would
// notice the difference.
//
// PHP arrays are values. `foreach ($rows as $row)` hands the body a copy, so
// `$row["x"] = 1` edits the copy and leaves `$rows` alone; `foreach ($rows as
// &$row)` hands it the element itself. phpscript's arrays are pointers, so the
// copy has to be made rather than deferred — there is no refcount to make it
// lazy the way PHP's copy-on-write does.
//
// Copying every element of every loop would be a large price for a semantic
// almost no loop uses, so the copy is made only when the body actually assigns
// through the loop variable. AssignsTo answers that, once per loop rather than
// once per iteration, and the flatstack compiler asks it once per program.

// CopyValue returns a value with PHP's assignment semantics applied: arrays are
// values and are copied, everything else is a handle or immutable and is
// returned as it is.
//
// The copy reaches nested arrays, because they are values too — PHP's
// `$copy["a"]["b"] = 1` cannot be observed through the original. It stops at
// objects, which are handles in PHP as well, and at the native Go collections a
// binding returns, which belong to the host rather than to the script.
func CopyValue(v any) any {
	array, ok := v.(*Array)
	if !ok {
		return v
	}
	return CopyArray(array)
}

// CopyArray returns an independent copy of a, with nested arrays copied too.
func CopyArray(a *Array) *Array {
	if a == nil {
		return nil
	}
	out := NewArraySize(a.Len())
	a.Range(func(key, val any) bool {
		out.Set(key, CopyValue(val))
		return true
	})
	return out
}

// AssignsTo reports whether any statement in body writes through the name that
// target is rooted at — `$v = …`, `$v["k"] = …`, `$v->p = …`, `$v++`, or a
// list() destructuring naming it.
//
// It is deliberately a root-name test rather than an exact-shape one. A write
// to `$v["k"]` has to count: the element it reaches lives inside the value the
// loop variable holds, so a by-value loop must have copied it. Over-reporting
// costs a copy that turns out to be unobservable; under-reporting would let a
// by-value loop edit its source.
func AssignsTo(body []Stmt, target Expr) bool {
	root := RootName(target)
	if root == "" {
		// A target with no name to root on cannot be matched against, so assume
		// the body writes to it.
		return true
	}
	return stmtsAssignTo(body, root)
}

// RootName returns the variable name an lvalue is rooted at: `$v`, `$v["k"]`
// and `$v->p->q` are all rooted at "v". It is "" for anything else.
func RootName(e Expr) string {
	for {
		switch n := UnwrapParenthesized(e).(type) {
		case *Var:
			if n.Const {
				return ""
			}
			return n.Name
		case *Index:
			e = n.Base
		case *PropAccess:
			e = n.Base
		default:
			return ""
		}
	}
}

func stmtsAssignTo(stmts []Stmt, root string) bool {
	for _, s := range stmts {
		if stmtAssignsTo(s, root) {
			return true
		}
	}
	return false
}

func stmtAssignsTo(s Stmt, root string) bool {
	switch n := s.(type) {
	case *Assign:
		return RootName(n.Target) == root || listAssignsTo(n.Target, root) || exprAssignsTo(n.Value, root)
	case *ExprStmt:
		return exprAssignsTo(n.X, root)
	case *Echo:
		return exprsAssignTo(n.Args, root)
	case *If:
		return exprAssignsTo(n.Cond, root) || stmtsAssignTo(n.Then, root) || stmtsAssignTo(n.Else, root)
	case *For:
		return stmtAssignsTo(n.Init, root) || exprAssignsTo(n.Cond, root) ||
			stmtAssignsTo(n.Post, root) || stmtsAssignTo(n.Body, root)
	case *Foreach:
		// A nested loop that binds the same name rebinds it on every iteration,
		// which is a write as far as the enclosing loop is concerned.
		return RootName(n.ValTarget) == root || RootName(n.KeyTarget) == root ||
			n.ValVar == root || n.KeyVar == root ||
			exprAssignsTo(n.Source, root) || stmtsAssignTo(n.Body, root)
	case *Switch:
		if exprAssignsTo(n.Cond, root) || stmtsAssignTo(n.Default, root) {
			return true
		}
		for _, c := range n.Cases {
			if exprAssignsTo(c.Value, root) || stmtsAssignTo(c.Body, root) {
				return true
			}
		}
	case *Try:
		if stmtsAssignTo(n.Body, root) || stmtsAssignTo(n.Finally, root) {
			return true
		}
		for _, c := range n.Catches {
			if c.Var == root || stmtsAssignTo(c.Body, root) {
				return true
			}
		}
	case *Return:
		return exprAssignsTo(n.Value, root)
	case *Throw:
		return exprAssignsTo(n.X, root)
	case *Unset:
		for _, t := range n.Targets {
			if RootName(t) == root {
				return true
			}
		}
	case *Include:
		// An included file is not visible here, and it shares this scope.
		return true
	}
	return false
}

// listAssignsTo reports whether a list() destructuring target names root.
func listAssignsTo(target Expr, root string) bool {
	list, ok := UnwrapParenthesized(target).(*ListExpr)
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

func exprsAssignTo(xs []Expr, root string) bool {
	for _, x := range xs {
		if exprAssignsTo(x, root) {
			return true
		}
	}
	return false
}

func exprAssignsTo(e Expr, root string) bool {
	switch n := e.(type) {
	case nil:
		return false
	case *AssignExpr:
		return RootName(n.Target) == root || listAssignsTo(n.Target, root) || exprAssignsTo(n.Value, root)
	case *Unary:
		// `$v++` and `--$v` write; every other unary only reads.
		if (n.Op == "++" || n.Op == "--") && RootName(n.X) == root {
			return true
		}
		return exprAssignsTo(n.X, root)
	case *Parenthesized:
		return exprAssignsTo(n.X, root)
	case *Binary:
		return exprAssignsTo(n.Left, root) || exprAssignsTo(n.Right, root)
	case *Ternary:
		return exprAssignsTo(n.Cond, root) || exprAssignsTo(n.Then, root) || exprAssignsTo(n.Else, root)
	case *Cast:
		return exprAssignsTo(n.X, root)
	case *Index:
		return exprAssignsTo(n.Base, root) || exprAssignsTo(n.Index, root)
	case *PropAccess:
		return exprAssignsTo(n.Base, root)
	case *MethodCall:
		return exprAssignsTo(n.Base, root) || exprsAssignTo(n.Args, root)
	case *Call:
		// A shim with an output parameter writes back into the frame, and the
		// by-reference table is not visible from here.
		return exprsAssignTo(n.Args, root) || callWritesBack(n, root)
	case *StaticCall:
		return exprsAssignTo(n.Args, root)
	case *Invoke:
		return exprAssignsTo(n.Callee, root) || exprsAssignTo(n.Args, root)
	case *New:
		return exprsAssignTo(n.Args, root)
	case *ArrayLit:
		for _, it := range n.Items {
			if exprAssignsTo(it.Key, root) || exprAssignsTo(it.Val, root) {
				return true
			}
		}
	case *ListExpr:
		return exprsAssignTo(n.Elems, root)
	case *Closure:
		// A closure captures by value, so its body cannot write to this frame.
		return false
	case *Include:
		return true
	}
	return false
}

// callWritesBack reports whether a call passes root to an output parameter.
func callWritesBack(n *Call, root string) bool {
	for _, arg := range n.Args {
		if v, ok := UnwrapParenthesized(arg).(*Var); ok && !v.Const && v.Name == root {
			return true
		}
	}
	return false
}
