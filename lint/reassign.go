package lint

import (
	"fmt"

	"github.com/titpetric/phpscript/model"
)

// lintTypeReassign reports a variable whose first literal assignment fixed a
// type and whose later literal assignment spells a different one. The first
// literal is read as the declaration of the variable's type, the convention
// the flat VM's typed operator selection leans on; a reassignment at another
// type is legal PHP and runs, which is why the finding is a warning rather
// than fatal.
//
// Tracking is per function body (methods included), and the whole body is one
// scope: an if arm assigning a string where the else arm assigns an int is
// the same drift the rule exists to name. Only assignments whose right side
// is a literal participate; a call, a variable or any other computed value
// says nothing about the declared type and neither records nor conflicts.
// Closure bodies are not walked.
func lintTypeReassign(file string, prog *model.Program, out *[]Diagnostic) {
	w := &reassignWalker{file: file, prog: prog, out: out}
	w.scope(prog.Stmts)
	for _, s := range prog.Stmts {
		w.declarations(s)
	}
	for _, decl := range prog.AnonClasses {
		w.classDecl(decl)
	}
}

type reassignWalker struct {
	file string
	prog *model.Program
	out  *[]Diagnostic
}

// scope checks one function body's statements against a fresh type table.
func (w *reassignWalker) scope(stmts []model.Stmt) {
	types := map[string]string{}
	w.walk(stmts, types)
}

// declarations opens a new scope for every function and method body found at
// any nesting of s, without re-checking the surrounding statements.
func (w *reassignWalker) declarations(s model.Stmt) {
	switch n := s.(type) {
	case *model.FuncDecl:
		w.scope(n.Body)
	case *model.ClassDecl:
		w.classDecl(n)
	case *model.If:
		w.declarationsAll(n.Then)
		w.declarationsAll(n.Else)
	case *model.For:
		w.declarationsAll(n.Body)
	case *model.Foreach:
		w.declarationsAll(n.Body)
	case *model.DoWhile:
		w.declarationsAll(n.Body)
	case *model.Try:
		w.declarationsAll(n.Body)
		for _, c := range n.Catches {
			w.declarationsAll(c.Body)
		}
		w.declarationsAll(n.Finally)
	case *model.Switch:
		for _, c := range n.Cases {
			w.declarationsAll(c.Body)
		}
		w.declarationsAll(n.Default)
	}
}

func (w *reassignWalker) declarationsAll(stmts []model.Stmt) {
	for _, s := range stmts {
		w.declarations(s)
	}
}

func (w *reassignWalker) classDecl(n *model.ClassDecl) {
	for _, m := range n.Methods {
		if m != nil {
			w.scope(m.Body)
		}
	}
}

// walk visits every statement of one scope, branches included, recording and
// checking literal assignments. Function and class declarations are skipped
// here; declarations opens their own scope.
func (w *reassignWalker) walk(stmts []model.Stmt, types map[string]string) {
	for _, s := range stmts {
		switch n := s.(type) {
		case *model.Assign:
			w.assign(n, types)
		case *model.If:
			w.walk(n.Then, types)
			w.walk(n.Else, types)
		case *model.For:
			if n.Init != nil {
				w.walk([]model.Stmt{n.Init}, types)
			}
			if n.Post != nil {
				w.walk([]model.Stmt{n.Post}, types)
			}
			w.walk(n.Body, types)
		case *model.Foreach:
			w.walk(n.Body, types)
		case *model.DoWhile:
			w.walk(n.Body, types)
		case *model.Try:
			w.walk(n.Body, types)
			for _, c := range n.Catches {
				w.walk(c.Body, types)
			}
			w.walk(n.Finally, types)
		case *model.Switch:
			for _, c := range n.Cases {
				w.walk(c.Body, types)
			}
			w.walk(n.Default, types)
		}
	}
}

func (w *reassignWalker) assign(n *model.Assign, types map[string]string) {
	if n.Op != "" && n.Op != "=" {
		return
	}
	target, ok := model.UnwrapParenthesized(n.Target).(*model.Var)
	if !ok {
		return
	}
	class := literalClass(model.UnwrapParenthesized(n.Value))
	if class == "" {
		return
	}
	previous, seen := types[target.Name]
	if !seen {
		types[target.Name] = class
		return
	}
	if previous == class {
		return
	}
	*w.out = append(*w.out, Diagnostic{
		File:    w.file,
		Line:    w.prog.SourceSpans[n].Start,
		Message: fmt.Sprintf("no reassignment: $%s previously declared as %s", target.Name, previous),
		// The runtime throws a RuntimeException for the same write, so the
		// finding is a promise, not advice, and the lint run fails on it.
		Fatal: true,
	})
}

// literalClass names the type a literal spells, "" for anything that is not
// a literal. null fixes no type: PHP initialises with null and assigns the
// real value later, and the rule has nothing to say about that.
func literalClass(e model.Expr) string {
	switch v := e.(type) {
	case *model.Interp:
		return "string"
	case *model.ArrayLit:
		return "array"
	case *model.Lit:
		switch v.Value.(type) {
		case string:
			return "string"
		case int, int64:
			return "int"
		case float64:
			return "float"
		case bool:
			return "bool"
		}
	case *model.Unary:
		if !v.Postfix && (v.Op == "-" || v.Op == "+") {
			switch literalClass(model.UnwrapParenthesized(v.X)) {
			case "int":
				return "int"
			case "float":
				return "float"
			}
		}
	}
	return ""
}
