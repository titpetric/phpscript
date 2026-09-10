package runner

import (
	"io"
	"testing"

	"github.com/titpetric/phpscript/model"
	"github.com/titpetric/phpscript/runner/expr"
)

// The benchmarks below pin the cost of the expr-lang seam before it moves
// behind runner/expr: the compile pipeline, the hoisted config, the compile
// caches, and Eval per transpiled node shape. They are the before/after
// evidence for any engine change; docs/allocation-performance.md describes
// how a run is taken.

// benchExprRuntime builds a runtime with just enough of a function table to
// exercise the type env: names that collide with expr's own builtins plus a
// plain binding, the same set the compile guard tests use.
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

// benchCompileSrc is transpiled once from a compound expression so the compile
// benchmarks measure the source shape Eval actually produces, not hand-written
// expr syntax.
func benchCompileSrc(b *testing.B) string {
	b.Helper()
	e := benchNestedExpr()
	tr := NewTranspiler()
	src, _, err := tr.Transpile(e)
	if err != nil {
		b.Fatalf("Transpile: %v", err)
	}
	return src
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

// BenchmarkExprCompileCold measures the parse/check/optimize/compile pipeline
// against the cached config, bypassing both program caches: the cost of the
// first sighting of a source string. The nature cache inside the config fills
// on the first iteration and stays, which is also what a warm runtime carries.
func BenchmarkExprCompileCold(b *testing.B) {
	rt := benchExprRuntime()
	src := benchCompileSrc(b)
	rt.mu.Lock()
	cfg := rt.exprConfig()
	rt.mu.Unlock()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := expr.CompileWith(src, cfg); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkExprConfig measures rebuilding the compile config after a
// function-table change: the reflective type-env walk that exprConfig exists
// to amortise. RegisterFunc is in the loop because it is what invalidates the
// config; its own cost is two map writes and a counter.
func BenchmarkExprConfig(b *testing.B) {
	rt := benchExprRuntime()
	fn := func(args ...any) any { return nil }

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		rt.RegisterFunc("strlen", fn)
		rt.mu.Lock()
		rt.exprConfig()
		rt.mu.Unlock()
	}
}

// BenchmarkExprCacheHit measures compile on a warm per-runtime cache: the
// mutex and the map lookup every repeated evaluation of a source pays.
func BenchmarkExprCacheHit(b *testing.B) {
	rt := benchExprRuntime()
	src := benchCompileSrc(b)
	if _, err := rt.compile(src); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := rt.compile(src); err != nil {
			b.Fatal(err)
		}
	}
}

// benchEval evaluates e once to fill every cache layer, then measures the
// steady state: env acquire, variable layering, and the VM run.
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
