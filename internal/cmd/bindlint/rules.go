package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

// checkSignature reports a binding parameter declared as a runtime value type.
//
// Arguments arrive as `any`. A concrete parameter makes reflect.Call panic the
// moment a script passes the `[]string` another binding returned, and the
// binding is unreachable for every value the interpreter did not build.
func (p *pkg) checkSignature(b binding) []finding {
	if b.ftype == nil || b.ftype.Params == nil {
		return nil
	}
	var out []finding
	for _, field := range b.ftype.Params.List {
		if paramType(field.Type) == "" {
			continue
		}
		name := types.ExprString(field.Type)
		file, line := p.position(field.Pos())
		out = append(out, finding{
			file:    file,
			line:    line,
			binding: b.label(),
			message: fmt.Sprintf("parameter %s is %s: a script value arrives as any, and a concrete "+
				"parameter makes reflect.Call panic on the []string another binding returned. Take any "+
				"and read it through model.RangeValues.", paramLabel(field), name),
		})
	}
	return out
}

// paramLabel names the parameter a finding is about, as a script spells it.
func paramLabel(field *ast.Field) string {
	if len(field.Names) == 0 {
		return "the unnamed argument"
	}
	return "$" + field.Names[0].Name
}

// checkNarrowing reports a binding that narrows a script value to a runtime
// value type for no reason, in its own body or in a helper of its package it
// passes the value to.
func (p *pkg) checkNarrowing(b binding) []finding {
	out := p.narrowings(b.label(), b.ftype, b.body)
	for _, decl := range p.reachable(b.body) {
		out = append(out, p.narrowings(b.label()+" via "+decl.Name.Name, decl.Type, decl.Body)...)
	}
	return out
}

// narrowings reports the sites in one function that narrow a value to a
// runtime value type, less the four that have a reason to.
//
// The value is the same value either way: `array("a" => 1)` reaches a binding
// as a *model.Array and the map another binding built reaches it as a
// map[string]any. A narrowing that only reads the value answers one of them
// and silently takes the other path for the other.
//
// It has a reason when the type switch routes the native shape to a case of
// its own, when the function reads the narrowed value's source through model
// or reflect whatever shape it has, when the branch taking what the narrowing
// did not match refuses it with an error - the loud refusal array_splice and
// the array mutators document - or when it calls for a capability a Go-native
// value has not got: in-place growth, which a slice behind an interface cannot
// do, and the typed storage.
//
// Each reason reads the narrowing it excuses and never the function holding
// it. A reader somewhere in the body says nothing about the value narrowed
// here, and `password_hash` proves the cost: its options bag reached an error
// built for another argument, and a function-wide reading of the refusal left
// the skipped bag unreported.
func (p *pkg) narrowings(label string, ftype *ast.FuncType, body *ast.BlockStmt) []finding {
	if body == nil {
		return nil
	}
	sites := narrowingSites(body)
	if len(sites) == 0 {
		return nil
	}

	var out []finding
	for _, s := range sites {
		if s.native || s.refused {
			continue
		}
		if s.subject != "" && readsEveryShape(body, s.subject) {
			continue
		}
		if s.bound != "" && usesCapability(body, s.bound) {
			continue
		}
		file, line := p.position(s.pos)
		out = append(out, finding{
			file:    file,
			line:    line,
			binding: label,
			message: fmt.Sprintf("narrows %s to %s and only reads it: the same value from another binding "+
				"is a Go slice or map and takes the other path. Read it through model.RangeValues or "+
				"model.LenValues, or refuse it with an error.", valueLabel(s.bound), s.typ),
		})
	}
	return out
}

// valueLabel names the value a narrowing is about.
func valueLabel(bound string) string {
	if bound == "" {
		return "a script value"
	}
	return bound
}

// site is one narrowing: the position, the type it narrows to, the variable
// holding the value it narrows, the variable the narrowed value lands in,
// whether a sibling case of the same type switch answers for the Go-native
// shape, and whether the branch taking the other shapes refuses them.
type site struct {
	pos     token.Pos
	typ     string
	subject string
	bound   string
	native  bool
	refused bool
}

// narrowingSites collects the type assertions and type switch cases in a
// function body that name a runtime value type.
func narrowingSites(body *ast.BlockStmt) []site {
	var out []site
	// A type switch and an assertion in an assignment both hold a
	// TypeAssertExpr the walk reaches again on its own; taken records the ones
	// already read, with the variables they name.
	taken := map[token.Pos]bool{}
	refusals := errorBranches(body)
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.TypeSwitchStmt:
			// A switch case takes no refusal: the native shape reaches a case
			// of its own or it reaches none, and the default clause answers
			// for every type the function never considered.
			out = append(out, switchSites(node, taken)...)
		case *ast.AssignStmt:
			if s, ok := assignSite(node, taken); ok {
				s.refused = refusals[s.pos]
				out = append(out, s)
			}
		case *ast.TypeAssertExpr:
			if taken[node.Pos()] {
				return true
			}
			if vmType(node.Type) != "" {
				taken[node.Pos()] = true
				out = append(out, site{
					pos:     node.Pos(),
					typ:     types.ExprString(node.Type),
					subject: subjectName(node.X),
					refused: refusals[node.Pos()],
				})
			}
		}
		return true
	})
	return out
}

// assignSite reads `a, ok := value.(*model.Array)`, the shape a narrowing
// takes when the binding keeps the narrowed value.
func assignSite(assign *ast.AssignStmt, taken map[token.Pos]bool) (site, bool) {
	if len(assign.Rhs) != 1 || len(assign.Lhs) == 0 {
		return site{}, false
	}
	assert, ok := assign.Rhs[0].(*ast.TypeAssertExpr)
	if !ok {
		return site{}, false
	}
	if vmType(assert.Type) == "" {
		return site{}, false
	}
	taken[assert.Pos()] = true
	return site{
		pos:     assert.Pos(),
		typ:     types.ExprString(assert.Type),
		subject: subjectName(assert.X),
		bound:   boundName(assign.Lhs[0]),
	}, true
}

// subjectName is the variable holding the value a narrowing reads:
// `opts[1].(*model.Array)` narrows opts, and `value.(*model.Object)` narrows
// value. An expression rooted in no name answers "".
func subjectName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.IndexExpr:
		return subjectName(x.X)
	case *ast.SelectorExpr:
		return subjectName(x.X)
	case *ast.ParenExpr:
		return subjectName(x.X)
	case *ast.StarExpr:
		return subjectName(x.X)
	}
	return ""
}

// switchSites reads the cases of a type switch, and whether any of them
// answers for a Go-native collection.
func switchSites(sw *ast.TypeSwitchStmt, taken map[token.Pos]bool) []site {
	bound := ""
	switch assign := sw.Assign.(type) {
	case *ast.AssignStmt:
		if len(assign.Lhs) > 0 {
			bound = boundName(assign.Lhs[0])
		}
		if len(assign.Rhs) == 1 {
			if assert, ok := assign.Rhs[0].(*ast.TypeAssertExpr); ok {
				taken[assert.Pos()] = true
			}
		}
	case *ast.ExprStmt:
		if assert, ok := assign.X.(*ast.TypeAssertExpr); ok {
			taken[assert.Pos()] = true
		}
	}

	native := false
	for _, stmt := range sw.Body.List {
		clause, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		for _, t := range clause.List {
			if nativeShape(t) {
				native = true
			}
		}
	}

	var out []site
	for _, stmt := range sw.Body.List {
		clause, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		for _, t := range clause.List {
			if vmType(t) != "" {
				out = append(out, site{pos: t.Pos(), typ: types.ExprString(t), bound: bound, native: native})
			}
		}
	}
	return out
}

// boundName is the variable an assertion binds, and "" for the blank one.
func boundName(e ast.Expr) string {
	ident, ok := e.(*ast.Ident)
	if !ok || ident.Name == "_" {
		return ""
	}
	return ident.Name
}

// reachable returns the functions of the package a binding calls, and the
// functions those call: a binding passes its argument to a helper, and the
// narrowing is the binding's wherever the file holding it sits.
func (p *pkg) reachable(body *ast.BlockStmt) []*ast.FuncDecl {
	if body == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []*ast.FuncDecl
	queue := []*ast.BlockStmt{body}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		for _, name := range calledNames(next) {
			if seen[name] {
				continue
			}
			seen[name] = true
			for _, decl := range p.funcs[name] {
				if decl.Body == nil {
					continue
				}
				out = append(out, decl)
				queue = append(queue, decl.Body)
			}
		}
	}
	return out
}

// calledNames lists the names a function body calls, in source order.
func calledNames(body *ast.BlockStmt) []string {
	var out []string
	ast.Inspect(body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if name := calledName(call); name != "" {
				out = append(out, name)
			}
		}
		return true
	})
	return out
}

// readsEveryShape reports whether a function reads the narrowed value's
// source whatever shape it has: through model's collection helpers, phpval, or
// reflection. The argument decides it, and not the call: a function reading
// one value through model.RangeValues and narrowing another answers for the
// first alone.
func readsEveryShape(body *ast.BlockStmt, subject string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || !nativeReaders[calledName(call)] {
			return true
		}
		for _, arg := range call.Args {
			if subjectName(arg) == subject {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// errorBranches maps the position of a narrowing to whether the branch taking
// what it did not match refuses the value with an error, which reaches the
// script as a throw. That is the shape arrayTarget and array_splice document:
// a mutator needs the one shape that can grow and says so.
//
// The branch is the one beside the narrowing: `if !ok` after the assignment,
// or the if statement the assertion sits in the init of. An error anywhere
// else in the function belongs to another argument.
func errorBranches(body *ast.BlockStmt) map[token.Pos]bool {
	out := map[token.Pos]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt:
			if assign, ok := node.Init.(*ast.AssignStmt); ok {
				markRefusal(out, assign, []ast.Stmt{node})
			}
		case *ast.BlockStmt:
			for i, stmt := range node.List {
				if assign, ok := stmt.(*ast.AssignStmt); ok {
					markRefusal(out, assign, node.List[i+1:])
				}
			}
		}
		return true
	})
	return out
}

// markRefusal records the narrowing in assign as refused when one of the
// statements that follow it tests the assertion's ok and builds an error.
func markRefusal(out map[token.Pos]bool, assign *ast.AssignStmt, rest []ast.Stmt) {
	if len(assign.Rhs) != 1 || len(assign.Lhs) < 2 {
		return
	}
	assert, ok := assign.Rhs[0].(*ast.TypeAssertExpr)
	if !ok || vmType(assert.Type) == "" {
		return
	}
	okName := boundName(assign.Lhs[1])
	if okName == "" || len(rest) == 0 {
		return
	}
	branch, isIf := rest[0].(*ast.IfStmt)
	if !isIf || !mentions(branch.Cond, okName) {
		return
	}
	if createsError(branch.Body) || createsError(branch.Else) {
		out[assert.Pos()] = true
	}
}

// mentions reports whether an expression reads the named variable.
func mentions(e ast.Expr, name string) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok && ident.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

// createsError reports whether a branch builds an error of its own, which is
// the refusal a script catches. An error it received from elsewhere belongs to
// the call that made it.
func createsError(branch ast.Node) bool {
	if branch == nil {
		return false
	}
	found := false
	ast.Inspect(branch, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		switch {
		case pkg.Name == "fmt" && sel.Sel.Name == "Errorf",
			pkg.Name == "errors" && (sel.Sel.Name == "New" || sel.Sel.Name == "Errorf"):
			found = true
			return false
		}
		return true
	})
	return found
}

// usesCapability reports whether a function calls a method on the narrowed
// value that a Go-native value cannot answer.
func usesCapability(body *ast.BlockStmt, bound string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if receiverName(call) == bound && vmCapabilities[calledName(call)] {
			found = true
			return false
		}
		return true
	})
	return found
}
