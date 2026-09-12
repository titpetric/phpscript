package runner

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner/expr"
)

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
