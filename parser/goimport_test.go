package parser

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// goImportHeader opens a file that may write an import: the go runtime binds
// the statement, as it binds the builtins.
const goImportHeader = "<?php\nuse Runtime\\Go;\n"

// lastExpr returns the expression the last statement of src compiled to.
func lastExpr(t *testing.T, src string) model.Expr {
	t.Helper()
	prog := mustParse(t, src)
	last := prog.Stmts[len(prog.Stmts)-1]
	stmt, ok := last.(*model.ExprStmt)
	if !ok {
		t.Fatalf("%s: last statement is a %T, want an expression", src, last)
	}
	return stmt.X
}

// TestGoImportStatement covers what `import "net/http"` records: the area that
// binds the package, the name a script addresses it by, and the path for
// whoever prints the file back.
func TestGoImportStatement(t *testing.T) {
	t.Parallel()
	prog := mustParse(t, goImportHeader+"import \"net/http\";\nimport \"encoding/json\";\n")
	var got []model.UseImport
	for _, s := range prog.Stmts {
		if use, ok := s.(*model.Use); ok {
			got = append(got, use.Imports...)
		}
	}
	// The `use` of the go runtime is the first of the three.
	want := []model.UseImport{
		{Path: `Runtime\Go`, Alias: ""},
		{Path: `Runtime\Go\Net\HTTP`, Alias: "http", Package: "net/http"},
		{Path: `Runtime\Go\Encoding\JSON`, Alias: "json", Package: "encoding/json"},
	}
	if len(got) != len(want) {
		t.Fatalf("the file imports %v, want %v", got, want)
	}
	for i, imp := range got {
		if imp != want[i] {
			t.Errorf("import %d is %v, want %v", i, imp, want[i])
		}
	}
}

// TestGoImportMember covers the one place a dot is a name: a member of a
// package the file imported, in each position a name takes.
func TestGoImportMember(t *testing.T) {
	t.Parallel()
	src := goImportHeader + "import \"net/http\";\n"
	for _, test := range []struct{ expr, want string }{
		{"http.NewRequest(\"GET\", $u);", `Runtime\Go\Net\HTTP\NewRequest`},
		{"http.StatusOK;", `Runtime\Go\Net\HTTP\StatusOK`},
		{"new http.Request();", `Runtime\Go\Net\HTTP\Request`},
		{"make([]http.Request, 2);", `[]Runtime\Go\Net\HTTP\Request`},
	} {
		switch n := lastExpr(t, src+test.expr).(type) {
		case *model.Call:
			if n.Name == `Runtime\Go\make` {
				lit, ok := n.Args[0].(*model.Lit)
				if !ok || lit.Value != test.want {
					t.Errorf("%s: make takes %v, want %s", test.expr, n.Args[0], test.want)
				}
				continue
			}
			if n.Name != test.want {
				t.Errorf("%s: calls %s, want %s", test.expr, n.Name, test.want)
			}
		case *model.Var:
			if !n.Const || n.Name != test.want {
				t.Errorf("%s: reads %s, want the constant %s", test.expr, n.Name, test.want)
			}
		case *model.New:
			if n.Class != test.want {
				t.Errorf("%s: constructs %s, want %s", test.expr, n.Class, test.want)
			}
		default:
			t.Errorf("%s: compiled to a %T", test.expr, n)
		}
	}
}

// TestGoImportInterpolation covers the {$...} form, which re-enters the parser
// on the file's package map: a member reads the same inside a string as it does
// outside one.
func TestGoImportInterpolation(t *testing.T) {
	t.Parallel()
	prog := mustParse(t, goImportHeader+"import \"net/http\";\n$n = 200;\necho \"{$n + http.StatusOK}\";")
	echo, ok := prog.Stmts[len(prog.Stmts)-1].(*model.Echo)
	if !ok || len(echo.Args) != 1 {
		t.Fatalf("the file compiles to %#v, want one echo", prog.Stmts[len(prog.Stmts)-1])
	}
	interp, ok := echo.Args[0].(*model.Interp)
	if !ok || len(interp.Parts) != 1 {
		t.Fatalf("the string compiles to %#v, want one interpolated part", echo.Args[0])
	}
	sum, ok := interp.Parts[0].(*model.Binary)
	if !ok {
		t.Fatalf("the interpolation compiles to %T, want the sum", interp.Parts[0])
	}
	name, ok := sum.Right.(*model.Var)
	if !ok || !name.Const || name.Name != `Runtime\Go\Net\HTTP\StatusOK` {
		t.Errorf("the sum reads %v, want the constant Runtime\\Go\\Net\\HTTP\\StatusOK", sum.Right)
	}
}

// TestGoImportRefusals covers what stays php's: the statement in a file that
// bound no go runtime, a path the standard library does not spell, and a dot
// an import does not claim.
func TestGoImportRefusals(t *testing.T) {
	t.Parallel()
	// A file with no go runtime reads `import` as an ordinary name, and the
	// dot concatenates the two operands.
	if call, ok := lastExpr(t, "<?php\nimport(\"net/http\");").(*model.Call); !ok || call.Name != "import" {
		t.Errorf("import is not a call to php's own name: %T", lastExpr(t, "<?php\nimport(\"net/http\");"))
	}
	// A module path reaches no area, and the parse says which line asked.
	_, err := Parse(goImportHeader + "import \"github.com/titpetric/phpscript\";")
	if err == nil || !strings.Contains(err.Error(), "no go package path") {
		t.Errorf("a module path parsed: %v", err)
	}
	// The statement is file level: the runtime installs the area from the
	// top-level statements before the file runs. The parser refuses a nested
	// one, which would name an area the run never installs.
	_, err = Parse(goImportHeader + "if (false) {\nimport \"net/http\";\n}")
	if err == nil || !strings.Contains(err.Error(), "only allowed at the top level") {
		t.Errorf("a nested import parsed: %v", err)
	}
	// A dot with no member behind it stays concatenation, in a file that
	// imported the package named on its left.
	call, ok := lastExpr(t, goImportHeader+"import \"net/http\";\nvar_dump(http . 1);").(*model.Call)
	if !ok {
		t.Fatal("var_dump is no call")
	}
	if _, ok := call.Args[0].(*model.Binary); !ok {
		t.Errorf("`http . 1` compiled to a %T, want the concatenation", call.Args[0])
	}
}
