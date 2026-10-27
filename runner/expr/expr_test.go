package expr

import (
	"fmt"
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// concatHelpers is the helper table a string concatenation reads. It fills
// no other table and a chain that reaches another helper panics on a nil func
// and the test names it.
func concatHelpers() *Helpers {
	return &Helpers{
		Concat: func(a, b any) string { return fmt.Sprint(a) + fmt.Sprint(b) },
	}
}

// TestProgram covers the two halves one compiled expression holds: the
// closure chain Run calls, and the slot table the runner resolves its
// variable list against. An expression reading no variable compiles to an
// empty table and evaluating it reads no Vars slice.
func TestProgram(t *testing.T) {
	t.Parallel()
	compiled, err := CompileExpr(&model.Lit{Value: int64(7)}, concatHelpers())
	if err != nil {
		t.Fatalf("CompileExpr: %v", err)
	}
	if compiled.Program == nil {
		t.Fatal("CompileExpr returned no Program")
	}
	if got := compiled.Program.NumSlots(); got != 0 {
		t.Errorf("NumSlots() = %d for a literal, want 0", got)
	}

	value, err := Run(compiled.Program, &Env{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if value != int64(7) {
		t.Errorf("Run() = %#v, want int64(7)", value)
	}
}

// TestRun covers evaluation against the runner's environment, and the single
// recover the chain carries. The guard sits at the root and not at each
// helper call and a panic anywhere down the chain surfaces as the error
// PanicError built, and a host panic stays catchable as one PHP exception on
// both engines.
func TestRun(t *testing.T) {
	t.Parallel()
	expression := &model.Binary{
		Op:    ".",
		Left:  &model.Var{Name: "greeting"},
		Right: &model.Var{Name: "name"},
	}
	values := map[string]any{"greeting": "hello ", "name": "ada"}

	compiled, err := CompileExpr(expression, concatHelpers())
	if err != nil {
		t.Fatalf("CompileExpr: %v", err)
	}

	env := &Env{Vars: make([]any, compiled.Program.NumSlots())}
	for _, variable := range compiled.Vars {
		env.Vars[variable.Slot] = values[variable.Name]
	}
	value, err := Run(compiled.Program, env)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if value != "hello ada" {
		t.Errorf("Run() = %#v, want %q", value, "hello ada")
	}

	// Run reports a helper that panics mid-chain through PanicError, with no
	// value beside the error.
	panicking := &Helpers{
		Concat:     func(a, b any) string { panic("nil map") },
		PanicError: func(recovered any) error { return fmt.Errorf("host panic: %v", recovered) },
	}
	compiled, err = CompileExpr(expression, panicking)
	if err != nil {
		t.Fatalf("CompileExpr: %v", err)
	}
	value, err = Run(compiled.Program, &Env{Vars: make([]any, compiled.Program.NumSlots())})
	if err == nil || !strings.Contains(err.Error(), "host panic: nil map") {
		t.Fatalf("Run() error = %v, want the error PanicError built", err)
	}
	if value != nil {
		t.Errorf("Run() = %#v beside an error, want nil", value)
	}
}

// TestProgram_Slots covers the identifier spelling the table keys on.
// The compiler reuses the transpiler's prefixes, `v_` for a PHP variable and
// `c_` for a bare name and `$FOO` and `FOO` take separate slots. One
// identifier read twice shares the slot it got the first time and the runner
// resolves a name once per expression.
func TestProgram_Slots(t *testing.T) {
	t.Parallel()
	expression := &model.Binary{
		Op: ".",
		Left: &model.Binary{
			Op:    ".",
			Left:  &model.Var{Name: "a"},
			Right: &model.Var{Name: "a"},
		},
		Right: &model.Var{Name: "PHP_EOL", Const: true},
	}

	compiled, err := CompileExpr(expression, concatHelpers())
	if err != nil {
		t.Fatalf("CompileExpr: %v", err)
	}

	slots := compiled.Program.Slots()
	if len(slots) != 2 {
		t.Fatalf("Slots() = %v, want one entry per distinct identifier", slots)
	}
	if got, ok := slots["v_a"]; !ok || got != 0 {
		t.Errorf("Slots()[v_a] = %d, %t, want 0, true", got, ok)
	}
	if got, ok := slots["c_PHP_EOL"]; !ok || got != 1 {
		t.Errorf("Slots()[c_PHP_EOL] = %d, %t, want 1, true", got, ok)
	}
}

// TestProgram_NumSlots covers the size Run reads the Vars slice at: one entry
// per distinct identifier and every slot read lands inside the slice. It
// matches the length of Vars, which is the list the runner fills.
func TestProgram_NumSlots(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		expr model.Expr
		want int
	}{
		{
			name: "literal",
			expr: &model.Lit{Value: "x"},
			want: 0,
		},
		{
			name: "one variable",
			expr: &model.Var{Name: "a"},
			want: 1,
		},
		{
			name: "one variable read twice",
			expr: &model.Binary{Op: ".", Left: &model.Var{Name: "a"}, Right: &model.Var{Name: "a"}},
			want: 1,
		},
		{
			name: "two variables",
			expr: &model.Binary{Op: ".", Left: &model.Var{Name: "a"}, Right: &model.Var{Name: "b"}},
			want: 2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			compiled, err := CompileExpr(test.expr, concatHelpers())
			if err != nil {
				t.Fatalf("CompileExpr: %v", err)
			}
			if got := compiled.Program.NumSlots(); got != test.want {
				t.Errorf("NumSlots() = %d, want %d", got, test.want)
			}
			if got := len(compiled.Vars); got != test.want {
				t.Errorf("len(Vars) = %d, want %d", got, test.want)
			}
		})
	}
}
