package runner

import (
	"io"
	"maps"
	"testing"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/checker/nature"
	"github.com/expr-lang/expr/conf"

	"github.com/titpetric/phpscript/model"
)

// TestTypeEnvNatureMatchesExprEnv pins the invariant behind typeEnvNature: the
// nature it builds by hand must be the one conf.EnvWithCache derives by walking
// the same map reflectively. TestCompileMatchesExprEnv covers the consequence
// (identical bytecode); this covers the cause, so a change in expr's env
// derivation is reported here rather than as a mystery compile difference.
func TestTypeEnvNatureMatchesExprEnv(t *testing.T) {
	rt := New(io.Discard, Options{})
	rt.RegisterFunc("strlen", func(args ...any) any { return nil })
	rt.RegisterFunc("count", func(args ...any) any { return nil })
	env := rt.typeEnvBase()

	var mine, theirs nature.Cache
	got := typeEnvNature(&mine, env)
	want := conf.EnvWithCache(&theirs, env)

	if got.Type != want.Type || got.Kind != want.Kind || got.Strict != want.Strict {
		t.Fatalf("nature = %v/%v/strict=%v, want %v/%v/strict=%v",
			got.Type, got.Kind, got.Strict, want.Type, want.Kind, want.Strict)
	}
	if len(got.Fields) != len(want.Fields) || len(got.Fields) != len(env) {
		t.Fatalf("fields = %d, want %d (env has %d)", len(got.Fields), len(want.Fields), len(env))
	}
	for name := range env {
		g, gok := got.Get(&mine, name)
		w, wok := want.Get(&theirs, name)
		if !gok || !wok {
			t.Fatalf("Get(%q) = %v/%v, want both found", name, gok, wok)
		}
		if g.Type != w.Type || g.Kind != w.Kind || g.Method != w.Method || g.Nil != w.Nil {
			t.Fatalf("field %q = %v/%v, want %v/%v", name, g.Type, g.Kind, w.Type, w.Kind)
		}
		// The memoised, type-derived facts the checker asks for must agree, since
		// every field shares one TypeData in the hand-built nature.
		if g.NumIn() != w.NumIn() || g.NumOut() != w.NumOut() || g.IsVariadic() != w.IsVariadic() {
			t.Fatalf("field %q signature = in %d/out %d/variadic %v, want in %d/out %d/variadic %v",
				name, g.NumIn(), g.NumOut(), g.IsVariadic(), w.NumIn(), w.NumOut(), w.IsVariadic())
		}
		if a, b := g.Out(&mine, 0), w.Out(&theirs, 0); a.Type != b.Type {
			t.Fatalf("field %q returns %v, want %v", name, a.Type, b.Type)
		}
	}
	if _, ok := got.Get(&mine, "no_such_name"); ok {
		t.Fatal("Get on an unregistered name: want not found")
	}
}

// TestCompileMatchesExprEnv pins the invariant behind the cached compile
// configuration: compiling with a config built once per function-table
// generation, with PHP variables left undefined, must produce exactly the
// bytecode expr.Compile produces when handed a full type env per call.
//
// The comparison is the disassembled program, so any difference in operator
// resolution, deref insertion or call opcode selection fails here.
func TestCompileMatchesExprEnv(t *testing.T) {
	rt := New(io.Discard, Options{})
	// A function table with names that collide with expr's own builtins and
	// predicate syntax (count, filter, map, len, sum), which is why the type env
	// still carries the function table at all.
	for _, name := range []string{"count", "filter", "map", "len", "sum", "strlen", "implode", "usort", "preg_match", "sprintf"} {
		rt.RegisterFunc(name, func(args ...any) any { return nil })
	}

	v := func(name string) model.Expr { return &model.Var{Name: name} }
	lit := func(val any) model.Expr { return &model.Lit{Value: val} }

	exprs := []model.Expr{
		v("a"),
		lit("s"),
		&model.Binary{Op: ".", Left: v("a"), Right: lit(" b")},
		&model.Binary{Op: "+", Left: v("a"), Right: v("b")},
		&model.Binary{Op: "===", Left: v("a"), Right: lit(1)},
		&model.Binary{Op: "===", Left: lit("x"), Right: lit("y")},
		&model.Binary{Op: "===", Left: lit(1), Right: lit(2)},
		&model.Binary{Op: "<", Left: v("a"), Right: lit(3)},
		&model.Binary{Op: "&&", Left: v("a"), Right: v("b")},
		&model.Unary{Op: "!", X: v("a")},
		&model.Unary{Op: "-", X: v("a")},
		&model.Ternary{Cond: v("a"), Then: lit(1), Else: v("b")},
		&model.Index{Base: v("a"), Index: lit("k")},
		&model.Index{Base: &model.Index{Base: v("a"), Index: lit(0)}, Index: v("i")},
		&model.PropAccess{Base: v("obj"), Name: "field"},
		&model.MethodCall{Base: v("obj"), Method: "run", Args: []model.Expr{v("a")}},
		&model.New{Class: "Foo", Args: []model.Expr{v("a")}},
		&model.Cast{Type: "int", X: v("a")},
		&model.ClassConst{Class: "Foo", Name: "BAR"},
		&model.AssignExpr{Target: v("a"), Op: "=", Value: lit(1)},
		&model.ArrayLit{Items: []model.ArrayItem{{Val: v("a")}, {Key: lit("k"), Val: lit(2)}}},
		&model.Call{Name: "count", Args: []model.Expr{v("a")}},
		&model.Call{Name: "filter", Args: []model.Expr{v("a")}},
		&model.Call{Name: "map", Args: []model.Expr{v("a"), v("b")}},
		&model.Call{Name: "len", Args: []model.Expr{v("a")}},
		&model.Call{Name: "sum", Args: []model.Expr{v("a")}},
		&model.Call{Name: "strlen", Args: []model.Expr{&model.Binary{Op: ".", Left: v("a"), Right: v("b")}}},
		&model.Call{Name: "preg_match", Args: []model.Expr{lit("/x/"), v("s"), v("m")}},
		&model.Call{Name: `App\f`, Fallback: "f", Args: []model.Expr{v("a")}},
		&model.Call{Name: "usort", Args: []model.Expr{v("a"), &model.Closure{}}},
	}

	// dyn is the sentinel the type env used to carry for every PHP variable: a
	// pointer to an empty interface, which expr dereferences to "any".
	dyn := new(any)

	for _, e := range exprs {
		tr := NewTranspiler()
		src, vars, err := tr.Transpile(e)
		if err != nil {
			t.Fatalf("Transpile: %v", err)
		}

		base := rt.typeEnvBase()
		typeEnv := make(map[string]any, len(base)+len(vars))
		maps.Copy(typeEnv, base)
		for id := range tr.Closures() {
			typeEnv[id] = typeEnvStub
		}
		for _, name := range vars {
			typeEnv[varIdent(name)] = dyn
		}

		want, err := expr.Compile(src, expr.Env(typeEnv), expr.DisableAllBuiltins())
		if err != nil {
			t.Fatalf("expr.Compile %q: %v", src, err)
		}
		got, err := compileWith(src, rt.exprConfig())
		if err != nil {
			t.Fatalf("compileWith %q: %v", src, err)
		}
		if a, b := want.Disassemble(), got.Disassemble(); a != b {
			t.Errorf("bytecode differs for %q\n--- expr.Env ---\n%s--- cached config ---\n%s", src, a, b)
		}
	}
}

// TestCompileConfigIsRebuiltOnRegistration asserts that a function registered
// after the first compile is visible to the next one — the config is cached per
// function-table generation, not forever.
func TestCompileConfigIsRebuiltOnRegistration(t *testing.T) {
	rt := New(io.Discard, Options{})
	first := rt.exprConfig()
	if same := rt.exprConfig(); same != first {
		t.Fatal("exprConfig rebuilt without a function-table change")
	}
	rt.RegisterFunc("count", func(args ...any) any { return nil })
	next := rt.exprConfig()
	if next == first {
		t.Fatal("exprConfig not rebuilt after RegisterFunc")
	}
	if _, ok := next.Env.Get(&next.NtCache, "count"); !ok {
		t.Fatal("count missing from the rebuilt compile config")
	}
}

// TestCompileUndefinedFunctionIsARuntimeError documents the one behavioural
// difference from compiling with a full type env: a call to a function that is
// not registered is no longer rejected at compile time. PHP reports the same
// condition at runtime ("call to undefined function"), so the error simply
// moves from Compile to Run.
func TestCompileUndefinedFunctionIsARuntimeError(t *testing.T) {
	rt := New(io.Discard, Options{})
	prog, err := rt.compile("no_such_function(v_x)")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if _, err := expr.Run(prog, map[string]any{"v_x": 1}); err == nil {
		t.Fatal("running a call to an undefined function: want error, got nil")
	}
}
