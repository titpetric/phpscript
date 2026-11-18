package lint

import (
	"fmt"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/modelutil"
)

// lintTypeReassign reports, as advice, a variable later assigned a different literal type.
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

// scope checks a body's statements, flagging string offsets on its declared strings.
func (w *reassignWalker) scope(stmts []model.Stmt) {
	types := map[string]string{}
	w.walk(stmts, types)
	offsets := &astWalker{prog: w.prog, skipDecls: true}
	offsets.expr = func(e model.Expr, line int) {
		idx, ok := e.(*model.Index)
		if !ok {
			return
		}
		v, ok := modelutil.UnwrapParenthesized(idx.Base).(*model.Var)
		if !ok || types[v.Name] != "string" {
			return
		}
		*w.out = append(*w.out, Diagnostic{
			File:    w.file,
			Line:    line,
			Message: fmt.Sprintf("string offset $%s[...]: offsets count bytes, so a multi-byte character reads as one of its bytes; mb_substr($%s, ...) spells a character read", v.Name, v.Name),
		})
	}
	offsets.walk(stmts)
}

// declarations opens a scope for each nested function and method body.
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

// walk visits every statement of one scope's branches, recording and checking literal assignments.
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
	target, ok := modelutil.UnwrapParenthesized(n.Target).(*model.Var)
	if !ok {
		return
	}
	class := literalClass(modelutil.UnwrapParenthesized(n.Value))
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
		// Advisory: the runtime follows PHP and retypes the variable and the
		// program runs. The finding flags the drift without failing the lint
		// run; keeping one type per name is the convention, which nothing enforces.
	})
}

// literalClass names the type a literal spells, "" for a non-literal and null. The rule stays quiet about that.
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
			switch literalClass(modelutil.UnwrapParenthesized(v.X)) {
			case "int":
				return "int"
			case "float":
				return "float"
			}
		}
	}
	return ""
}
