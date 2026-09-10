package runner

import (
	"io"
	"reflect"
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestClosureEngineMatchesVM holds the two evaluation engines against each
// other: every corpus expression is evaluated through the closure chain and
// again through the bytecode VM on the same scope, and the values must agree.
// Errors are compared by presence, not text: the VM decorates its errors with
// source locations the closure engine does not reproduce.
//
// wantClosure additionally pins which shapes the closure engine must accept,
// so a regression that silently drops everything to the VM fails here rather
// than showing up as a benchmark surprise.
func TestClosureEngineMatchesVM(t *testing.T) {
	rt := New(io.Discard, Options{})
	rt.RegisterFunc("strlen", func(args ...any) any {
		s, _ := args[0].(string)
		return int64(len(s))
	})
	rt.RegisterFunc("count", func(args ...any) any {
		if a, ok := args[0].(*model.Array); ok {
			return int64(a.Len())
		}
		return int64(0)
	})

	scope := NewScope()
	scope.Set("a", int64(2))
	scope.Set("b", int64(3))
	scope.Set("c", int64(10))
	scope.Set("s", "hello")
	scope.Set("f", 1.5)
	scope.Set("t", true)
	arr := model.NewArray()
	arr.Set("k", int64(42))
	arr.Set(int64(0), "zero")
	scope.Set("arr", arr)

	v := func(name string) model.Expr { return &model.Var{Name: name} }
	lit := func(val any) model.Expr { return &model.Lit{Value: val} }

	cases := []struct {
		name        string
		expr        model.Expr
		wantClosure bool
	}{
		{"var", v("a"), true},
		{"undefined var", v("nope"), true},
		{"arith int", &model.Binary{Op: "+", Left: v("a"), Right: v("b")}, true},
		{"arith float", &model.Binary{Op: "*", Left: v("f"), Right: v("b")}, true},
		{"arith div", &model.Binary{Op: "/", Left: v("c"), Right: v("b")}, true},
		{"arith mod zero", &model.Binary{Op: "%", Left: v("a"), Right: lit(0)}, true},
		{"cmp identical", &model.Binary{Op: "===", Left: v("a"), Right: lit(2)}, true},
		{"cmp loose", &model.Binary{Op: "==", Left: v("a"), Right: lit("2")}, true},
		{"cmp lt", &model.Binary{Op: "<", Left: v("a"), Right: v("b")}, true},
		{"bit and", &model.Binary{Op: "&", Left: v("c"), Right: v("b")}, true},
		{"bit shift", &model.Binary{Op: "<<", Left: v("a"), Right: v("b")}, true},
		{"and", &model.Binary{Op: "&&", Left: v("t"), Right: v("a")}, true},
		{"or short", &model.Binary{Op: "||", Left: v("t"), Right: v("nope")}, true},
		{"not", &model.Unary{Op: "!", X: v("t")}, true},
		{"neg", &model.Unary{Op: "-", X: v("a")}, true},
		{"ternary then", &model.Ternary{Cond: v("t"), Then: v("a"), Else: v("b")}, true},
		{"ternary else", &model.Ternary{Cond: lit(0), Then: v("a"), Else: v("b")}, true},
		{"index string key", &model.Index{Base: v("arr"), Index: lit("k")}, true},
		{"index int key", &model.Index{Base: v("arr"), Index: lit(0)}, true},
		{"index missing", &model.Index{Base: v("arr"), Index: lit("missing")}, true},
		{"cast", &model.Cast{Type: "int", X: v("f")}, true},
		{"array literal", &model.ArrayLit{Items: []model.ArrayItem{{Val: v("a")}, {Key: lit("k"), Val: v("b")}}}, true},
		{"call binding", &model.Call{Name: "strlen", Args: []model.Expr{v("s")}}, true},
		{"call colliding name", &model.Call{Name: "count", Args: []model.Expr{v("arr")}}, true},
		{"call undefined", &model.Call{Name: "no_such_function", Args: []model.Expr{v("a")}}, true},
		{"prop on non-object", &model.PropAccess{Base: v("a"), Name: "field"}, true},
		{"method on non-object", &model.MethodCall{Base: v("a"), Method: "run"}, true},
		{"nested", &model.Ternary{
			Cond: &model.Binary{Op: "===", Left: &model.Binary{Op: "+", Left: v("a"), Right: v("b")}, Right: lit(5)},
			Then: &model.Binary{Op: "*", Left: v("c"), Right: lit(2)},
			Else: &model.Call{Name: "strlen", Args: []model.Expr{v("s")}},
		}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			closureOut, closureErr := rt.Eval(tc.expr, scope)

			ce, ok := rt.compiled[tc.expr]
			if !ok {
				t.Fatal("expression missing from the compiled cache after Eval")
			}
			if got := ce.prog.HasClosure(); got != tc.wantClosure {
				t.Fatalf("HasClosure = %v, want %v", got, tc.wantClosure)
			}

			prog := ce.prog
			ce.prog = prog.VMOnly()
			defer func() { ce.prog = prog }()
			vmOut, vmErr := rt.Eval(tc.expr, scope)

			if (closureErr == nil) != (vmErr == nil) {
				t.Fatalf("error mismatch: closure=%v vm=%v", closureErr, vmErr)
			}
			if !reflect.DeepEqual(closureOut, vmOut) {
				t.Fatalf("value mismatch: closure=%#v (%T) vm=%#v (%T)", closureOut, closureOut, vmOut, vmOut)
			}
		})
	}
}
