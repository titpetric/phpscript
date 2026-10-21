package expr

import (
	"fmt"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestDirectCompileBinary covers the binary sub-compiler. An arithmetic
// operator resolves through the Arith helper, `&&` and `||` short-circuit so
// the right operand of a decided expression never evaluates, and an operator
// outside the table is a compile error.
func TestDirectCompileBinary(t *testing.T) {
	t.Parallel()
	helpers := &Helpers{
		Truthy: func(v any) bool { return v == true },
		Arith: func(op string, a, b any) any {
			if op != "+" {
				t.Errorf("Arith saw op %q, want +", op)
			}
			return a.(int64) + b.(int64)
		},
	}

	sum := &model.Binary{Op: "+", Left: &model.Lit{Value: int64(2)}, Right: &model.Lit{Value: int64(3)}}
	compiled, err := CompileExpr(sum, helpers)
	if err != nil {
		t.Fatalf("CompileExpr: %v", err)
	}
	value, err := Run(compiled.Program, &Env{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if value != int64(5) {
		t.Errorf("Run() = %#v, want 5", value)
	}

	// The right operand is a call into the base env, so evaluating it with no
	// base entry errors: a short-circuited `&&` never reaches it.
	decided := &model.Binary{
		Op:    "&&",
		Left:  &model.Lit{Value: false},
		Right: &model.Call{Name: "boom"},
	}
	compiled, err = CompileExpr(decided, helpers)
	if err != nil {
		t.Fatalf("CompileExpr: %v", err)
	}
	value, err = Run(compiled.Program, &Env{})
	if err != nil {
		t.Fatalf("a short-circuited && evaluated its right operand: %v", err)
	}
	if value != false {
		t.Errorf("Run() = %#v, want false", value)
	}

	if _, err := CompileExpr(&model.Binary{Op: "??", Left: sum, Right: sum}, helpers); err == nil {
		t.Error("an unsupported binary operator compiled")
	}
}

// TestDirectCompileUnary covers the unary sub-compiler. `!` reads the Truthy
// helper, `-` the Negate helper, unary `+` is the numeric cast 0 + $x through
// Arith, and an operator outside the table is a compile error.
func TestDirectCompileUnary(t *testing.T) {
	t.Parallel()
	helpers := &Helpers{
		Truthy: func(v any) bool { return v == true },
		Negate: func(v any) any { return -v.(int64) },
		Arith: func(op string, a, b any) any {
			return a.(int64) + b.(int64)
		},
	}

	for _, tc := range []struct {
		op    string
		value any
		want  any
	}{
		{"!", true, false},
		{"-", int64(5), int64(-5)},
		{"+", int64(7), int64(7)},
	} {
		compiled, err := CompileExpr(&model.Unary{Op: tc.op, X: &model.Lit{Value: tc.value}}, helpers)
		if err != nil {
			t.Fatalf("CompileExpr(%q): %v", tc.op, err)
		}
		got, err := Run(compiled.Program, &Env{})
		if err != nil {
			t.Fatalf("Run(%q): %v", tc.op, err)
		}
		if got != tc.want {
			t.Errorf("Run(%q) = %#v, want %#v", tc.op, got, tc.want)
		}
	}

	if _, err := CompileExpr(&model.Unary{Op: "?", X: &model.Lit{Value: int64(1)}}, helpers); err == nil {
		t.Error("an unsupported unary operator compiled")
	}
}

// TestDirectCompileInterp covers the interpolation sub-compiler. The parts
// write into one builder, so the result allocates once; a lone literal part
// is itself, and a literal with no parts is the empty string.
func TestDirectCompileInterp(t *testing.T) {
	t.Parallel()
	helpers := &Helpers{
		Str: func(v any) string { return fmt.Sprint(v) },
	}

	interp := &model.Interp{Parts: []model.Expr{
		&model.Lit{Value: "n="},
		&model.Var{Name: "n"},
	}}
	compiled, err := CompileExpr(interp, helpers)
	if err != nil {
		t.Fatalf("CompileExpr: %v", err)
	}
	env := &Env{Vars: make([]any, compiled.Program.NumSlots())}
	env.Vars[compiled.Vars[0].Slot] = int64(7)
	value, err := Run(compiled.Program, env)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if value != "n=7" {
		t.Errorf("Run() = %#v, want %q", value, "n=7")
	}

	lone, err := CompileExpr(&model.Interp{Parts: []model.Expr{&model.Lit{Value: "text"}}}, helpers)
	if err != nil {
		t.Fatalf("CompileExpr: %v", err)
	}
	if value, _ := Run(lone.Program, &Env{}); value != "text" {
		t.Errorf("a lone literal part evaluated to %#v, want itself", value)
	}

	empty, err := CompileExpr(&model.Interp{}, helpers)
	if err != nil {
		t.Fatalf("CompileExpr: %v", err)
	}
	if value, _ := Run(empty.Program, &Env{}); value != "" {
		t.Errorf("an empty literal evaluated to %#v, want the empty string", value)
	}
}
