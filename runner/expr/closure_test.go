package expr

import (
	"fmt"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestEnv covers the split the evaluation environment keeps. A per-expression
// value sits in Vars at the slot the compiler assigned to its identifier, and
// everything persistent sits in Base under its name. A registered function is
// a Base entry with the variadic signature the compiled call reads it through,
// and the compiler lists the names Eval installs there.
func TestEnv(t *testing.T) {
	t.Parallel()
	expression := &model.Call{
		Name: "upper",
		Args: []model.Expr{&model.Var{Name: "word"}},
	}

	compiled, err := CompileExpr(expression, &Helpers{})
	if err != nil {
		t.Fatalf("CompileExpr: %v", err)
	}
	if len(compiled.Calls) != 1 || compiled.Calls[0] != "upper" {
		t.Fatalf("Calls = %v, want the one name to install in Base", compiled.Calls)
	}
	if len(compiled.Vars) != 1 || compiled.Vars[0].Name != "word" {
		t.Fatalf("Vars = %+v, want the one variable the argument reads", compiled.Vars)
	}

	env := &Env{
		Vars: make([]any, compiled.Program.NumSlots()),
		Base: map[string]any{
			"upper": func(args ...any) (any, error) {
				return strings.ToUpper(fmt.Sprint(args[0])), nil
			},
		},
	}
	env.Vars[compiled.Vars[0].Slot] = "ada"

	value, err := Run(compiled.Program, env)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if value != "ADA" {
		t.Errorf("Run() = %#v, want %q", value, "ADA")
	}

	// A name with no Base entry is an error and never a nil call: the chain
	// reads the entry as func(...any) (any, error) and reports the name when
	// the type assertion misses.
	_, err = Run(compiled.Program, &Env{Vars: make([]any, compiled.Program.NumSlots())})
	if err == nil || !strings.Contains(err.Error(), "cannot call upper") {
		t.Errorf("Run() error = %v, want the missing Base entry named", err)
	}
}

// TestHelpers covers the dispatch the helper table carries: the compiled
// chain holds the typed implementation for the operator it compiled and calls
// it with the operands themselves, with no []any slice between them. One
// operator reaches one field, so a table filled for three operators records
// three separate calls.
func TestHelpers(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		op    string
		field string
		want  any
	}{
		{op: ".", field: "Concat", want: "12"},
		{op: "+", field: "Arith", want: int64(3)},
		{op: "==", field: "Compare", want: false},
	} {
		t.Run(test.op, func(t *testing.T) {
			var called string
			helpers := &Helpers{
				Concat: func(a, b any) string {
					called = "Concat"
					return fmt.Sprint(a) + fmt.Sprint(b)
				},
				Arith: func(op string, a, b any) any {
					called = "Arith"
					return a.(int64) + b.(int64)
				},
				Compare: func(op string, a, b any) bool {
					called = "Compare"
					return a == b
				},
			}

			compiled, err := CompileExpr(&model.Binary{
				Op:    test.op,
				Left:  &model.Lit{Value: int64(1)},
				Right: &model.Lit{Value: int64(2)},
			}, helpers)
			if err != nil {
				t.Fatalf("CompileExpr: %v", err)
			}

			value, err := Run(compiled.Program, &Env{})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if called != test.field {
				t.Errorf("the chain called %q, want %q", called, test.field)
			}
			if value != test.want {
				t.Errorf("Run() = %#v, want %#v", value, test.want)
			}
		})
	}
}
