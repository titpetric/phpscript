package lint

import (
	"fmt"
	"os"
	"strings"

	"github.com/titpetric/phpscript/annotations"
	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/list"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
	"github.com/titpetric/phpscript/tests"
)

// Diagnostic is one lint finding.
type Diagnostic struct {
	File    string
	Line    int
	Message string
}

func (d Diagnostic) String() string {
	if d.File != "" {
		return fmt.Sprintf("%s:%d: %s", d.File, d.Line, d.Message)
	}
	return fmt.Sprintf("line %d: %s", d.Line, d.Message)
}

// File parses and lints one PHP source file.
func File(name, src string) ([]Diagnostic, error) {
	src, err := lintSource(name, src)
	if err != nil {
		return []Diagnostic{{File: name, Line: 1, Message: fmt.Sprintf("fixture parse error: %v", err)}}, nil
	}
	prog, err := parser.Parse(src)
	if err != nil {
		return []Diagnostic{{File: name, Line: 1, Message: fmt.Sprintf("parse error: %v", err)}}, nil
	}
	var out []Diagnostic
	lintJSONFlags(name, src, &out)
	lintRoutes(name, src, &out)
	lintInterfaces(name, prog, &out)
	lintStmts(name, prog, &out)
	lintReferences(name, prog, &out)
	lintUndefinedNames(name, prog, &out)
	return out, nil
}

// lintJSONFlags reports a JSON_* constant.
//
// No JSON_* constant is defined here, so the name evaluates to null and
// json_encode ignores the argument. The call runs and encodes correctly, which
// is why this is a warning: nothing is broken, and the author has asked for a
// formatting the encoder does not vary. See docs/design.md.
//
// The scan is over tokens rather than the AST: a constant can appear in any
// expression, and T_STRING already tells a bare name apart from the same text
// inside a string literal, so defined("JSON_PRETTY_PRINT") is not a use.
func lintJSONFlags(file, src string, out *[]Diagnostic) {
	for _, token := range parser.TokenGetAll(src) {
		triple, ok := token.([]any)
		if !ok || len(triple) != 3 {
			continue
		}
		// A token id and line are int64, the type a PHP integer has here.
		id, _ := triple[0].(int64)
		text, _ := triple[1].(string)
		line, _ := triple[2].(int64)
		if id != int64(parser.T_STRING) || !strings.HasPrefix(text, "JSON_") {
			continue
		}
		*out = append(*out, Diagnostic{
			File: file,
			Line: int(line),
			Message: fmt.Sprintf("%s is not defined and the argument is ignored: the JSON encoding is not "+
				"configurable. Drop it, and indent downstream if the output has to be read; see docs/design.md.", text),
		})
	}
}

// lintRoutes reports an @route path whose parameters the router cannot
// answer for.
//
// The two routers this runtime registers on disagree about what a {...}
// segment may say, and neither refuses the spellings the other does not
// understand in a way an author sees: chi takes {module=users} as a parameter
// of that literal name, matches every request to the segment and exports
// nothing. Registration skips such a route; this is the diagnostic that names
// the file and the line before it gets that far.
func lintRoutes(file, src string, out *[]Diagnostic) {
	for _, route := range annotations.ParseRoutes([]byte(src)) {
		if _, err := model.ParseRoutePath(route.Path); err != nil {
			*out = append(*out, Diagnostic{File: file, Line: route.Line, Message: err.Error()})
		}
	}
}

// lintInterfaces reports a class that declares `implements` and does not
// declare a method the interface names. The runtime raises a RuntimeException
// for the same condition, so the finding is the warning that arrives before the
// program is run.
func lintInterfaces(file string, prog *model.Program, out *[]Diagnostic) {
	for _, v := range model.CheckInterfaces(prog.Stmts) {
		*out = append(*out, Diagnostic{
			File:    file,
			Line:    prog.SourceSpans[v.Decl].Start,
			Message: v.String(),
		})
	}
}

// FlatstackFile checks whether a single PHP source file is compatible with flatstack engine.
func FlatstackFile(name, src string) (Diagnostic, error) {
	src, err := lintSource(name, src)
	if err != nil {
		return Diagnostic{File: name, Line: 1, Message: fmt.Sprintf("fixture parse error: %v", err)}, nil
	}
	prog, err := parser.Parse(src)
	if err != nil {
		return Diagnostic{File: name, Line: 1, Message: fmt.Sprintf("parse error: %v", err)}, nil
	}
	if err := flatstack.Supports(prog); err != nil {
		return Diagnostic{File: name, Line: 1, Message: fmt.Sprintf("[flatstack unsupported] %v", err)}, nil
	}
	return Diagnostic{File: name, Line: 1, Message: "[flatstack compatible] 100% compatible with flatstack bytecode engine"}, nil
}

// FlatstackPaths checks flatstack compatibility for all files matched by paths.
func FlatstackPaths(paths []string) ([]Diagnostic, error) {
	files, err := list.ExpandFiles(paths)
	if err != nil {
		return nil, err
	}
	var out []Diagnostic
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		diag, err := FlatstackFile(file, string(b))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		out = append(out, diag)
	}
	return out, nil
}

// Paths lints each file selected by the provided Go-style path patterns.
func Paths(paths []string) ([]Diagnostic, error) {
	files, err := list.ExpandFiles(paths)
	if err != nil {
		return nil, err
	}
	var out []Diagnostic
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		diags, err := File(file, string(b))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		out = append(out, diags...)
	}
	return out, nil
}

func lintSource(name, src string) (string, error) {
	if !strings.HasSuffix(name, ".phpt") {
		return src, nil
	}

	fixture, err := tests.ParseFixture([]byte(src), name)
	if err != nil {
		return "", err
	}

	// Keep the PHP section at its physical position in the fixture so lint
	// diagnostics point to lines in the .phpt file rather than section-relative
	// lines.
	normalized := strings.ReplaceAll(src, "\r\n", "\n")
	phpStart := strings.Index(normalized, "\n---\n") + len("\n---\n")
	lineOffset := strings.Count(normalized[:phpStart], "\n")
	if strings.HasPrefix(normalized[phpStart:], "\n") {
		lineOffset++ // ParseFixture trims one optional leading section newline.
	}
	return strings.Repeat("\n", lineOffset) + fixture.PHP, nil
}

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

// lintGlobal reports the `global $x;` statement, a documented won't-implement
// (docs/design.md): it parses and binds nothing, so the variable reads as
// unset at a distance from the line that looks like it imports it.
func (w *stmtWalker) lintGlobal(n *model.Global) {
	*w.out = append(*w.out, Diagnostic{
		File:    w.file,
		Line:    w.prog.SourceSpans[n].Start,
		Message: "global is a no-op: the variable stays unset; pass the collaborator as a parameter",
	})
}

// lintAbstract reports the abstract modifier, which has nothing to mean
// without inheritance (docs/design.md): an abstract class can be instantiated
// like any other, and an abstract method has no body, so calling it returns
// null where PHP would refuse to load the class uncompleted. Both parse and
// are kept by the formatter; the linter is where the author hears that no
// contract is being enforced. An interface is the contract that is checked.
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
// __construct and __invoke run (docs/design.md, "Won't implement"); a class
// that declares __call, __get or any other implicit hook is dead code that
// looks load-bearing, which is worse than absent.
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

// lintExtends reports `extends` on a class, a documented won't-implement
// (docs/design.md): the parent name is recorded and nothing arrives through
// it, so the child answers only with the members it wrote and is not an
// instanceof the parent. An interface's extends is not reported: there it
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

// lintReferences reports the reference markers that parse and confer nothing
// (docs/design.md, "`&` outside `foreach`"): `$a = &$b` binds by value, and
// `function &f()` returns by value. Both spellings survive the formatter, so
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

// lintChainedAssign reports `$a = $b = value`, where one value is bound to two
// or more names in a single statement.
//
// PHP copies an array on assignment, so there the two names end up holding
// independent arrays. phpscript's arrays are references, so both names see one
// array and a later write through either is visible through the other, a bug
// the shape hides rather than announces.
//
// The rule only sees the chains that are left after the parser has fixed the
// ones it can. A chain ending in an array literal is split into one allocation
// per name (parser.lowerChainedAlloc) and never reaches here; a chain ending in
// a scalar literal is left alone and skipped here, because a string or a number
// has no interior for the names to share. What remains is a chain ending in a
// name, a call or a `new`, where either the value is a handle the names really
// do share -- `$dba = $dbb = new Database` gives one connection two names in
// PHP as well -- or its type is not known until the statement runs. Both are
// worth a second look, which is what the finding asks for.
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

// chainedValue walks to the end of an assignment chain, so that
// `$a = $b = $c = '00'` is judged by the `'00'` rather than by the assignment
// that binds `$c`.
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
// those, which is how a negative number is written. A *model.Lit holding a
// *model.Array is not one of these; the parser does not build them, but the
// check reads the value rather than assuming it.
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
