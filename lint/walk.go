package lint

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// lintStmts applies the statement-shaped checks to every statement list in
// prog, at any nesting. The walker carries the file and program so the
// recursion does not thread them through every call.
func lintStmts(file string, prog *model.Program, out *[]Diagnostic) {
	w := &stmtWalker{file: file, prog: prog, out: out}
	w.walk(prog.Stmts)
	for _, decl := range prog.AnonClasses {
		w.lintMagicMethods(decl)
	}
}

type stmtWalker struct {
	file string
	prog *model.Program
	out  *[]Diagnostic
}

func (w *stmtWalker) walk(stmts []model.Stmt) {
	for _, s := range stmts {
		switch n := s.(type) {
		case *model.Assign:
			lintChainedAssign(w.file, n, w.out)
		case *model.Global:
			w.lintGlobal(n)
		case *model.If:
			lintCondition(w.file, n.Cond, w.out)
			w.walk(n.Then)
			w.walk(n.Else)
		case *model.For:
			if n.Init != nil {
				w.walk([]model.Stmt{n.Init})
			}
			if n.Cond != nil {
				lintCondition(w.file, n.Cond, w.out)
			}
			if n.Post != nil {
				w.walk([]model.Stmt{n.Post})
			}
			w.walk(n.Body)
		case *model.Foreach:
			w.walk(n.Body)
		case *model.DoWhile:
			lintCondition(w.file, n.Cond, w.out)
			w.walk(n.Body)
		case *model.FuncDecl:
			w.walk(n.Body)
		case *model.ClassDecl:
			w.lintExtends(n)
			w.lintAbstract(n)
			w.lintMagicMethods(n)
			for _, m := range n.Methods {
				w.walk(m.Body)
			}
		case *model.Try:
			w.walk(n.Body)
			for _, c := range n.Catches {
				w.walk(c.Body)
			}
			w.walk(n.Finally)
		case *model.Switch:
			for _, c := range n.Cases {
				w.walk(c.Body)
			}
			w.walk(n.Default)
		}
	}
}

// lintGlobal reports the `global $x;` statement, a documented won't-build
// (docs/design.md): it parses and binds nothing and the variable reads as
// unset at a distance from the line that looks like it imports it.
func (w *stmtWalker) lintGlobal(n *model.Global) {
	*w.out = append(*w.out, Diagnostic{
		File:    w.file,
		Line:    w.prog.SourceSpans[n].Start,
		Message: "global is a no-op: the variable stays unset; pass the collaborator as a parameter",
	})
}

// lintAbstract reports the abstract modifier, which has nothing to mean
// without inheritance (docs/design.md): an abstract class instantiates like
// any other, and an abstract method has no body and calling it returns null
// where PHP declines the class uncompleted. The parser takes both and the
// formatter keeps them; the linter is where the author hears that nothing
// enforces a contract. An interface is the contract the linter checks.
func (w *stmtWalker) lintAbstract(n *model.ClassDecl) {
	if n.Abstract {
		*w.out = append(*w.out, Diagnostic{
			File:    w.file,
			Line:    w.prog.SourceSpans[n].Start,
			Message: fmt.Sprintf("abstract is a no-op: %s can be instantiated; declare an interface for the contract (docs/design.md)", n.Name),
		})
	}
	for _, m := range n.Methods {
		if !m.Abstract {
			continue
		}
		line := w.prog.SourceSpans[n].Start
		if span, ok := w.prog.SourceSpans[m]; ok {
			line = span.Start
		}
		*w.out = append(*w.out, Diagnostic{
			File:    w.file,
			Line:    line,
			Message: fmt.Sprintf("abstract method %s::%s() is a no-op: it has no body and a call returns null; declare the body (docs/design.md)", n.Name, m.Name),
		})
	}
}

// lintMagicMethods reports a magic method the runtime never calls. Only
// __construct and __invoke run (docs/design.md, "Won't build"); a class
// that declares __call, __get or any other implicit hook is dead code that
// reads as if something depended on it, which is worse than absent.
func (w *stmtWalker) lintMagicMethods(n *model.ClassDecl) {
	for _, m := range n.Methods {
		if !strings.HasPrefix(m.Name, "__") {
			continue
		}
		switch strings.ToLower(m.Name) {
		case "__construct", "__invoke":
			continue
		}
		line := w.prog.SourceSpans[n].Start
		if span, ok := w.prog.SourceSpans[m]; ok {
			line = span.Start
		}
		*w.out = append(*w.out, Diagnostic{
			File: w.file,
			Line: line,
			Message: fmt.Sprintf("magic method %s::%s() is never called implicitly: only __construct and __invoke run; "+
				"declare an explicit method (docs/design.md)", n.Name, m.Name),
		})
	}
}

// lintExtends reports `extends` on a class, a documented won't-build
// (docs/design.md): the parser records the parent name and nothing arrives
// through it and the child answers only with the members it wrote and is not
// an instanceof the parent. An interface's extends is not reported: there it
// widens the declaration contract, which instanceof does follow.
func (w *stmtWalker) lintExtends(n *model.ClassDecl) {
	if n.Parent == "" {
		return
	}
	*w.out = append(*w.out, Diagnostic{
		File:    w.file,
		Line:    w.prog.SourceSpans[n].Start,
		Message: fmt.Sprintf("extends is a no-op: %s inherits nothing from %s; declare the members it uses", n.Name, n.Parent),
	})
}
