package lint

import (
	"fmt"
	"strings"

	"github.com/titpetric/phpscript/model"
)

// lintRedeclared reports a function name declared twice in one file, and one
// declared over a name the runtime already registers. Both are fatal errors in
// PHP and a catchable RedeclareError here, so the finding is the warning that
// arrives before the program is run.
//
// A name the source guards with function_exists is skipped: that is the
// polyfill idiom, where declaring over an absent built-in is the whole point,
// and the author has already said they handle both cases.
//
// Declarations at any nesting count. The runtime only honours the ones written
// at the top level of a file, so a nested duplicate is a divergence rather than
// a collision today, but it is still the same mistake and PHP still refuses it.
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

	rt := knownRuntime()
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
			})
		case rt.FunctionExists(decl.Name):
			*out = append(*out, Diagnostic{
				File:    file,
				Line:    line,
				Message: fmt.Sprintf("Cannot redeclare function %s()", decl.Name),
			})
		default:
			seen[key] = line
		}
	}
	declarations.walk(prog.Stmts)
}
