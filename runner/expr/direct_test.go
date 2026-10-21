package expr

import (
	"strings"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestDirectCompile covers the node dispatcher: a literal is its own value, a
// ternary takes the branch its condition picks, an index resolves through the
// Index helper, and an expression shape outside the vocabulary is a compile
// error and never a silent nil.
func TestDirectCompile(t *testing.T) {
	t.Parallel()
	helpers := &Helpers{
		Truthy: func(v any) bool { return v == true },
		Index: func(base, idx any) any {
			return base.(map[string]any)[idx.(string)]
		},
	}

	ternary := &model.Ternary{
		Cond: &model.Lit{Value: true},
		Then: &model.Lit{Value: "yes"},
		Else: &model.Lit{Value: "no"},
	}
	compiled, err := CompileExpr(ternary, helpers)
	if err != nil {
		t.Fatalf("CompileExpr: %v", err)
	}
	value, err := Run(compiled.Program, &Env{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if value != "yes" {
		t.Errorf("a true condition evaluated to %#v, want the then branch", value)
	}

	index := &model.Index{
		Base:  &model.Var{Name: "row"},
		Index: &model.Lit{Value: "name"},
	}
	compiled, err = CompileExpr(index, helpers)
	if err != nil {
		t.Fatalf("CompileExpr: %v", err)
	}
	env := &Env{Vars: make([]any, compiled.Program.NumSlots())}
	env.Vars[compiled.Vars[0].Slot] = map[string]any{"name": "ada"}
	value, err = Run(compiled.Program, env)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if value != "ada" {
		t.Errorf("Run() = %#v, want the indexed value", value)
	}

	if _, err := CompileExpr(nil, helpers); err == nil || !strings.Contains(err.Error(), "unsupported expression") {
		t.Errorf("an unsupported expression compiled: %v", err)
	}
}

// TestDirectCompileCall covers call compilation. A plain global call resolves
// its name in the base env and records it in Calls so Eval installs it; a
// namespaced name and a call carrying a global fallback dispatch through
// __func, so the runtime's resolution ladder answers both spellings.
func TestDirectCompileCall(t *testing.T) {
	t.Parallel()
	plain := &model.Call{Name: "greet", Args: []model.Expr{&model.Lit{Value: "ada"}}}
	compiled, err := CompileExpr(plain, &Helpers{})
	if err != nil {
		t.Fatalf("CompileExpr: %v", err)
	}
	if len(compiled.Calls) != 1 || compiled.Calls[0] != "greet" {
		t.Fatalf("Calls = %v, want the plain name recorded", compiled.Calls)
	}
	env := &Env{Base: map[string]any{
		"greet": func(args ...any) (any, error) { return "hello " + args[0].(string), nil },
	}}
	value, err := Run(compiled.Program, env)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if value != "hello ada" {
		t.Errorf("Run() = %#v, want the call result", value)
	}

	namespaced := &model.Call{Name: `App\greet`, Fallback: "greet"}
	compiled, err = CompileExpr(namespaced, &Helpers{})
	if err != nil {
		t.Fatalf("CompileExpr: %v", err)
	}
	if len(compiled.Calls) != 0 {
		t.Errorf("a namespaced call recorded %v, want dispatch through __func", compiled.Calls)
	}
	var gotName, gotFallback any
	env = &Env{Base: map[string]any{
		"__func": func(args ...any) (any, error) {
			gotName, gotFallback = args[0], args[1]
			return nil, nil
		},
	}}
	if _, err := Run(compiled.Program, env); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotName != `App\greet` || gotFallback != "greet" {
		t.Errorf("__func saw (%#v, %#v), want the name and its fallback", gotName, gotFallback)
	}
}
