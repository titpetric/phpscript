package lint

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner"
	"github.com/titpetric/phpscript/stdlib"
)

// The undefined-name check compares every call and class reference in a file
// against two universes: the names the file declares itself, and the names a
// runtime would answer for after stdlib registration. A miss is a warning, not
// a failure — a name can arrive through an include or an autoloader the lint
// pass does not chase — but each finding is a line that would raise
// "call to undefined function" or "undefined class" the moment it runs, so it
// is worth reading before the runtime says it.

var (
	registryOnce sync.Once
	registry     *runner.Runtime
)

// knownRuntime is the registry the checks query for host-provided names. It is
// built once per process: registration is deterministic, and the runtime never
// executes a script here, so the table is read-only after construction.
func knownRuntime() *runner.Runtime {
	registryOnce.Do(func() {
		rt := runner.New(io.Discard, runner.Options{SAPI: "cli"})
		stdlib.Register(rt)
		registry = rt
	})
	return registry
}

// declaredNames is what one file provides for itself: functions, classes and
// interfaces declared at any nesting (a conditional polyfill declaration
// counts), plus the names the source guards with function_exists /
// class_exists, which say the author already handles absence.
type declaredNames struct {
	funcs   map[string]bool
	classes map[string]bool
	guarded map[string]bool
}

func (d *declaredNames) hasFunc(name string) bool {
	key := strings.ToLower(name)
	return d.funcs[key] || d.guarded[key]
}

func (d *declaredNames) hasClass(name string) bool {
	key := strings.ToLower(name)
	return d.classes[key] || d.guarded[key]
}

// lintUndefinedNames reports function calls and class references nothing
// answers for. Messages mirror the runtime's own, so the finding reads as the
// error that would otherwise arrive at call time.
func lintUndefinedNames(file string, prog *model.Program, out *[]Diagnostic) {
	declared := &declaredNames{
		funcs:   map[string]bool{},
		classes: map[string]bool{},
		guarded: map[string]bool{},
	}
	collect := &astWalker{prog: prog}
	collect.expr = func(e model.Expr, _ int) {
		if call, ok := e.(*model.Call); ok {
			switch strings.ToLower(call.Name) {
			case "function_exists", "class_exists", "interface_exists", "method_exists":
				if len(call.Args) > 0 {
					if lit, ok := call.Args[0].(*model.Lit); ok {
						if name, ok := lit.Value.(string); ok {
							declared.guarded[strings.ToLower(name)] = true
						}
					}
				}
			}
		}
	}
	collect.stmt = func(s model.Stmt) {
		switch n := s.(type) {
		case *model.FuncDecl:
			if n.Class == "" {
				declared.funcs[strings.ToLower(n.Name)] = true
			}
		case *model.ClassDecl:
			declared.classes[strings.ToLower(n.Name)] = true
		case *model.InterfaceDecl:
			declared.classes[strings.ToLower(n.Name)] = true
		}
	}
	collect.walk(prog.Stmts)
	for _, decl := range prog.AnonClasses {
		declared.classes[strings.ToLower(decl.Name)] = true
	}

	rt := knownRuntime()
	report := func(line int, message string) {
		*out = append(*out, Diagnostic{File: file, Line: line, Message: message})
	}
	classKnown := func(class string) bool {
		// self/static/parent resolve against the running method's class, which
		// the walk does not track; the class they name is declared somewhere.
		switch class {
		case "self", "static", "parent":
			return true
		}
		if declared.hasClass(class) {
			return true
		}
		known, _ := rt.ClassExists(class, false)
		return known
	}

	check := &astWalker{prog: prog}
	check.expr = func(e model.Expr, line int) {
		switch n := e.(type) {
		case *model.Call:
			if n.Bare {
				return
			}
			if declared.hasFunc(n.Name) || rt.FunctionExists(n.Name) {
				return
			}
			if n.Fallback != "" && (declared.hasFunc(n.Fallback) || rt.FunctionExists(n.Fallback)) {
				return
			}
			report(line, fmt.Sprintf("call to undefined function %s()", n.Name))
		case *model.New:
			if n.Decl != nil {
				return // anonymous class, declared in place
			}
			if !classKnown(n.Class) {
				report(line, fmt.Sprintf("new: undefined class %q", n.Class))
			}
		case *model.StaticCall:
			if !classKnown(n.Class) {
				report(line, fmt.Sprintf("static call %s::%s(): unknown class", n.Class, n.Method))
			}
		case *model.StaticProp:
			if !classKnown(n.Class) {
				report(line, fmt.Sprintf("static property %s::$%s: unknown class", n.Class, n.Name))
			}
		case *model.ClassConst:
			// `Name::class` is the name as a string and needs no declaration.
			if n.Name != "class" && !classKnown(n.Class) {
				report(line, fmt.Sprintf("class constant %s::%s: unknown class", n.Class, n.Name))
			}
		}
	}
	check.walk(prog.Stmts)
}

// astWalker visits every statement and every expression in a statement list,
// at any nesting, including closure bodies written inside expressions. Each
// expression is reported with the source line of the statement holding it,
// since expressions carry no spans of their own.
type astWalker struct {
	prog *model.Program
	stmt func(model.Stmt)
	expr func(model.Expr, int)
	line int
}

func (w *astWalker) walk(stmts []model.Stmt) {
	for _, s := range stmts {
		if s == nil {
			continue
		}
		if span, ok := w.prog.SourceSpans[s]; ok {
			w.line = span.Start
		}
		if w.stmt != nil {
			w.stmt(s)
		}
		switch n := s.(type) {
		case *model.Echo:
			w.exprs(n.Args)
		case *model.ExprStmt:
			w.one(n.X)
		case *model.Assign:
			w.one(n.Target)
			w.one(n.Value)
		case *model.If:
			w.one(n.Cond)
			w.walk(n.Then)
			w.walk(n.Else)
		case *model.Foreach:
			w.one(n.Source)
			w.one(n.KeyTarget)
			w.one(n.ValTarget)
			w.walk(n.Body)
		case *model.For:
			if n.Init != nil {
				w.walk([]model.Stmt{n.Init})
			}
			w.one(n.Cond)
			if n.Post != nil {
				w.walk([]model.Stmt{n.Post})
			}
			w.walk(n.Body)
		case *model.Return:
			w.one(n.Value)
		case *model.Include:
			w.one(n.Path)
		case *model.FuncDecl:
			w.params(n.Params)
			w.walk(n.Body)
		case *model.ClassDecl:
			w.fields(n.Fields)
			w.fields(n.Statics)
			w.fields(n.Consts)
			for _, m := range n.Methods {
				w.params(m.Params)
				w.walk(m.Body)
			}
		case *model.InterfaceDecl:
			w.fields(n.Consts)
		case *model.Unset:
			w.exprs(n.Targets)
		case *model.Throw:
			w.one(n.X)
		case *model.Try:
			w.walk(n.Body)
			for _, c := range n.Catches {
				w.walk(c.Body)
			}
			w.walk(n.Finally)
		case *model.Switch:
			w.one(n.Cond)
			for _, c := range n.Cases {
				w.one(c.Value)
				w.walk(c.Body)
			}
			w.walk(n.Default)
		case *model.Declare:
			w.walk(n.Body)
		case *model.StaticVar:
			for _, d := range n.Vars {
				w.one(d.Default)
			}
		}
	}
}

func (w *astWalker) params(params []model.Param) {
	for _, p := range params {
		w.one(p.Default)
	}
}

func (w *astWalker) fields(fields []model.Field) {
	for _, f := range fields {
		w.one(f.Default)
	}
}

func (w *astWalker) exprs(list []model.Expr) {
	for _, e := range list {
		w.one(e)
	}
}

func (w *astWalker) one(e model.Expr) {
	if e == nil {
		return
	}
	if w.expr != nil {
		w.expr(e, w.line)
	}
	switch n := e.(type) {
	case *model.Interp:
		w.exprs(n.Parts)
	case *model.ArrayLit:
		for _, item := range n.Items {
			w.one(item.Key)
			w.one(item.Val)
		}
	case *model.Index:
		w.one(n.Base)
		w.one(n.Index)
	case *model.PropAccess:
		w.one(n.Base)
	case *model.Call:
		w.exprs(n.Args)
	case *model.MethodCall:
		w.one(n.Base)
		w.exprs(n.Args)
	case *model.New:
		w.exprs(n.Args)
		if n.Decl != nil {
			// An anonymous class declares its body inside the expression;
			// nothing else walks it. The declaration is offered to the
			// statement hook so class-shaped checks see it too.
			if w.stmt != nil {
				w.stmt(n.Decl)
			}
			w.fields(n.Decl.Fields)
			w.fields(n.Decl.Statics)
			w.fields(n.Decl.Consts)
			for _, m := range n.Decl.Methods {
				w.params(m.Params)
				w.walk(m.Body)
			}
		}
	case *model.Unary:
		w.one(n.X)
	case *model.Ref:
		w.one(n.X)
	case *model.Parenthesized:
		w.one(n.X)
	case *model.Binary:
		w.one(n.Left)
		w.one(n.Right)
	case *model.Ternary:
		w.one(n.Cond)
		w.one(n.Then)
		w.one(n.Else)
	case *model.Cast:
		w.one(n.X)
	case *model.Closure:
		w.params(n.Params)
		w.walk(n.Body)
	case *model.AssignExpr:
		w.one(n.Target)
		w.one(n.Value)
	case *model.ListExpr:
		w.exprs(n.Elems)
	case *model.StaticCall:
		w.one(n.MethodExpr)
		w.exprs(n.Args)
	case *model.StaticProp, *model.ClassConst, *model.Var, *model.Lit:
		// leaves
	case *model.Invoke:
		w.one(n.Callee)
		w.exprs(n.Args)
	case *model.Include:
		w.one(n.Path)
	}
}
