package lint

import (
	"fmt"
	"os"
	"strings"

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
	lintStmts(name, prog.Stmts, &out)
	return out, nil
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

func lintStmts(file string, stmts []model.Stmt, out *[]Diagnostic) {
	for _, s := range stmts {
		switch n := s.(type) {
		case *model.Assign:
			lintChainedAssign(file, n, out)
		case *model.If:
			lintCondition(file, n.Cond, out)
			lintStmts(file, n.Then, out)
			lintStmts(file, n.Else, out)
		case *model.For:
			if n.Init != nil {
				lintStmts(file, []model.Stmt{n.Init}, out)
			}
			if n.Cond != nil {
				lintCondition(file, n.Cond, out)
			}
			if n.Post != nil {
				lintStmts(file, []model.Stmt{n.Post}, out)
			}
			lintStmts(file, n.Body, out)
		case *model.Foreach:
			lintStmts(file, n.Body, out)
		case *model.FuncDecl:
			lintStmts(file, n.Body, out)
		case *model.ClassDecl:
			for _, m := range n.Methods {
				lintStmts(file, m.Body, out)
			}
		case *model.Try:
			lintStmts(file, n.Body, out)
			for _, c := range n.Catches {
				lintStmts(file, c.Body, out)
			}
			lintStmts(file, n.Finally, out)
		case *model.Switch:
			for _, c := range n.Cases {
				lintStmts(file, c.Body, out)
			}
			lintStmts(file, n.Default, out)
		}
	}
}

// lintChainedAssign reports `$a = $b = value`, where one value is bound to two
// or more names in a single statement.
//
// PHP copies an array on assignment, so there the two names end up holding
// independent arrays. phpscript's arrays are references, so both names see one
// array and a later write through either is visible through the other — which
// is a bug the shape hides rather than announces. Objects are handles in both
// languages and scalars are immutable in both, so for those the chain is only a
// readability question; the rule does not try to tell the cases apart, because
// the type of `value` is not known until the statement runs.
func lintChainedAssign(file string, n *model.Assign, out *[]Diagnostic) {
	chained, ok := model.UnwrapParenthesized(n.Value).(*model.AssignExpr)
	if !ok {
		return
	}
	*out = append(*out, Diagnostic{
		File:    file,
		Line:    chained.Line,
		Message: "chained assignment binds one value to several names",
	})
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
	case *model.Parenthesized:
		collectAssignExprs(n.X, out)
	case *model.Binary:
		collectAssignExprs(n.Left, out)
		collectAssignExprs(n.Right, out)
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
