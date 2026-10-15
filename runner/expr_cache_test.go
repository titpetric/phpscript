package runner

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner/expr"
)

// TestExprCache_GetExpr covers the read side of the per-node cache. The key is
// node identity and no structural equality: the cache is filled off the AST a
// program holds, and that AST is the one Eval walks again, so two equal nodes
// written in two places are two entries. A nil cache answers a miss, which is
// what lets a runtime with no cache evaluate without a check at every lookup.
func TestExprCache_GetExpr(t *testing.T) {
	t.Parallel()
	cache := NewExprCache()
	node := &model.Binary{Op: "+", Left: &model.Var{Name: "a"}, Right: &model.Lit{Value: int64(1)}}

	if _, ok := cache.GetExpr(node); ok {
		t.Fatal("GetExpr on an empty cache answered a hit")
	}

	compiled, err := expr.CompileExpr(node, exprHelpers)
	if err != nil {
		t.Fatal(err)
	}
	cache.SetExpr(node, compiled)

	got, ok := cache.GetExpr(node)
	if !ok {
		t.Fatal("GetExpr missed the node it was just given")
	}
	if got != compiled {
		t.Error("GetExpr answered a program other than the one stored")
	}

	twin := &model.Binary{Op: "+", Left: &model.Var{Name: "a"}, Right: &model.Lit{Value: int64(1)}}
	if _, ok := cache.GetExpr(twin); ok {
		t.Error("GetExpr matched an equal node, want identity")
	}

	var absent *ExprCache
	if _, ok := absent.GetExpr(node); ok {
		t.Error("GetExpr on a nil cache answered a hit")
	}
}

// TestExprCache_EnsureFlat covers the write-through compile a caller gates a
// flatstack run on. A program the compiler accepts is compiled once and read
// back, and one it refuses reports the refusal on every call, because nothing
// is stored for it: a stored verdict would let one run decide for the next.
func TestExprCache_EnsureFlat(t *testing.T) {
	t.Parallel()
	cache := NewExprCache()
	accepted := &model.Program{Stmts: []model.Stmt{&model.Echo{Args: []model.Expr{&model.Lit{Value: "ok"}}}}}

	if err := cache.EnsureFlat(accepted); err != nil {
		t.Fatalf("EnsureFlat: %v", err)
	}
	program, ok := cache.getFlat(accepted)
	if !ok {
		t.Fatal("EnsureFlat reported no error and stored nothing")
	}
	if err := cache.EnsureFlat(accepted); err != nil {
		t.Fatalf("second EnsureFlat: %v", err)
	}
	if again, _ := cache.getFlat(accepted); again != program {
		t.Error("the second EnsureFlat recompiled, want the table read back")
	}

	// A break outside a loop is a form the compiler refuses, which is the
	// verdict flatstack.Supports reports as "run this on the interpreter".
	refused := &model.Program{Stmts: []model.Stmt{&model.Break{Line: 1}}}
	first := cache.EnsureFlat(refused)
	if first == nil {
		t.Fatal("EnsureFlat accepted a break outside a loop")
	}
	if second := cache.EnsureFlat(refused); second == nil {
		t.Error("a refused program was accepted on the second call")
	}
	if _, ok := cache.getFlat(refused); ok {
		t.Error("a refused program was stored")
	}
}

// The benchmarks below pin the cost of the expression engine: compilation
// and Eval per node shape. They are the before/after evidence for any engine
// change; docs/allocation-performance.md describes how a run is taken and
// carries the measured history, including the expr-lang engine these
// replaced.

// benchExprRuntime builds a runtime with a small function table: names that
// collide with common builtins plus a plain binding.
func benchExprRuntime() *Runtime {
	rt := New(io.Discard, Options{})
	for _, name := range []string{"count", "filter", "map", "len", "sum", "implode"} {
		rt.RegisterFunc(name, func(args ...any) any { return nil })
	}
	rt.RegisterFunc("strlen", func(args ...any) any {
		s, _ := args[0].(string)
		return int64(len(s))
	})
	return rt
}

func benchNestedExpr() model.Expr {
	v := func(name string) model.Expr { return &model.Var{Name: name} }
	lit := func(val any) model.Expr { return &model.Lit{Value: val} }
	return &model.Ternary{
		Cond: &model.Binary{
			Op:    "===",
			Left:  &model.Binary{Op: "*", Left: &model.Binary{Op: "+", Left: v("a"), Right: v("b")}, Right: lit(2)},
			Right: v("c"),
		},
		Then: &model.Binary{Op: "+", Left: v("a"), Right: lit(1)},
		Else: &model.Call{Name: "strlen", Args: []model.Expr{&model.Binary{Op: ".", Left: v("s"), Right: lit("!")}}},
	}
}

// BenchmarkExprCompileDirect measures compiling a compound expression: one
// AST walk building the closure chain.
func BenchmarkExprCompileDirect(b *testing.B) {
	e := benchNestedExpr()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := expr.CompileExpr(e, exprHelpers); err != nil {
			b.Fatal(err)
		}
	}
}

// benchEval evaluates e once to fill every cache layer, then measures the
// steady state: env acquire, slot binding, and the closure run.
func benchEval(b *testing.B, e model.Expr) {
	b.Helper()
	rt := benchExprRuntime()
	scope := NewScope()
	scope.Set("a", int64(2))
	scope.Set("b", int64(3))
	scope.Set("c", int64(10))
	scope.Set("s", "hello")
	arr := model.NewArray()
	arr.Set("k", int64(42))
	scope.Set("arr", arr)

	if _, err := rt.Eval(e, scope); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := rt.Eval(e, scope); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEvalVar(b *testing.B) {
	benchEval(b, &model.Var{Name: "a"})
}

func BenchmarkEvalArith(b *testing.B) {
	benchEval(b, &model.Binary{Op: "+", Left: &model.Var{Name: "a"}, Right: &model.Var{Name: "b"}})
}

func BenchmarkEvalCompare(b *testing.B) {
	benchEval(b, &model.Binary{Op: "===", Left: &model.Var{Name: "a"}, Right: &model.Lit{Value: 1}})
}

func BenchmarkEvalTernary(b *testing.B) {
	benchEval(b, &model.Ternary{
		Cond: &model.Var{Name: "a"},
		Then: &model.Lit{Value: 1},
		Else: &model.Var{Name: "b"},
	})
}

func BenchmarkEvalIndex(b *testing.B) {
	benchEval(b, &model.Index{Base: &model.Var{Name: "arr"}, Index: &model.Lit{Value: "k"}})
}

func BenchmarkEvalCallBinding(b *testing.B) {
	benchEval(b, &model.Call{Name: "strlen", Args: []model.Expr{&model.Var{Name: "s"}}})
}

func BenchmarkEvalNested(b *testing.B) {
	benchEval(b, benchNestedExpr())
}

// BenchmarkEvalInterp prices a double-quoted literal that embeds expressions.
// The parts are what "user=$s k=$arr[k]!" parses to, so the number is the
// per-evaluation cost of the literal and not of the parse, which happens once.
func BenchmarkEvalInterp(b *testing.B) {
	benchEval(b, &model.Interp{Parts: []model.Expr{
		&model.Lit{Value: "user="},
		&model.Var{Name: "s"},
		&model.Lit{Value: " k="},
		&model.Index{Base: &model.Var{Name: "arr"}, Index: &model.Lit{Value: "k"}},
		&model.Lit{Value: "!"},
	}})
}
