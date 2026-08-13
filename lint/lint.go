package lint

import (
	"fmt"
	"os"

	"github.com/titpetric/phpscript/flatstack"
	"github.com/titpetric/phpscript/list"
	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/parser"
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
	prog, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	var out []Diagnostic
	lintStmts(name, prog.Stmts, &out)
	return out, nil
}

// FlatstackFile checks whether a single PHP source file is compatible with flatstack engine.
func FlatstackFile(name, src string) (Diagnostic, error) {
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

func lintStmts(file string, stmts []model.Stmt, out *[]Diagnostic) {
	for _, s := range stmts {
		switch n := s.(type) {
		case *model.If:
			lintCondition(file, n.Cond, out)
			lintStmts(file, n.Then, out)
			lintStmts(file, n.Else, out)
		case *model.For:
			if n.Cond != nil {
				lintCondition(file, n.Cond, out)
			}
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
