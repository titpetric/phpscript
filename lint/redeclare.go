package lint

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// lintRedeclared reports a duplicate or host-shadowing function name, skipping guarded declarations.
func lintRedeclared(file string, prog *model.Program, out *[]Diagnostic) {
	guarded := map[string]bool{}
	guards := &astWalker{prog: prog}
	guards.expr = func(e model.Expr, _ int) {
		call, ok := e.(*model.Call)
		if !ok || !strings.EqualFold(call.Name, "function_exists") || len(call.Args) == 0 {
			return
		}
		if lit, ok := call.Args[0].(*model.Lit); ok {
			if name, ok := lit.Value.(string); ok {
				guarded[strings.ToLower(name)] = true
			}
		}
	}
	guards.walk(prog.Stmts)

	seen := map[string]int{}
	declarations := &astWalker{prog: prog}
	declarations.stmt = func(s model.Stmt) {
		decl, ok := s.(*model.FuncDecl)
		if !ok || decl.Class != "" {
			return
		}
		key := strings.ToLower(decl.Name)
		if guarded[key] {
			return
		}
		line := declarations.line
		switch previous, repeated := seen[key]; {
		case repeated:
			*out = append(*out, Diagnostic{
				File:    file,
				Line:    line,
				Message: fmt.Sprintf("Cannot redeclare function %s() (previously declared on line %d)", decl.Name, previous),
				Fatal:   true,
			})
		case hostFunc(decl.Name):
			*out = append(*out, Diagnostic{
				File:    file,
				Line:    line,
				Message: fmt.Sprintf("Cannot redeclare function %s()", decl.Name),
				Fatal:   true,
			})
		default:
			seen[key] = line
		}
	}
	declarations.walk(prog.Stmts)
}
