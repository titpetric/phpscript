package runner

import (
	"testing"

	"github.com/titpetric/phpscript/model"
)

// TestTranspileSource pins the exact expr-lang source emitted for every node
// kind. The compiled program is cached by this string, so a change here is a
// change to the whole compile pipeline; the literals below are the reference.
func TestTranspileSource(t *testing.T) {
	v := func(name string) model.Expr { return &model.Var{Name: name} }
	lit := func(val any) model.Expr { return &model.Lit{Value: val} }

	cases := []struct {
		name string
		expr model.Expr
		want string
		vars []string
	}{
		{
			name: "literals",
			expr: &model.ArrayLit{Items: []model.ArrayItem{
				{Val: lit(nil)},
				{Val: lit(true)},
				{Val: lit(false)},
				{Val: lit("a\"b")},
				{Val: lit(7)},
				{Val: lit(int64(8))},
				{Val: lit(1.5)},
			}},
			want: `__array(__pair(nil, nil), __pair(nil, true), __pair(nil, false), __pair(nil, "a\"b"), __pair(nil, 7), __pair(nil, 8), __pair(nil, 1.5))`,
		},
		{
			name: "keyed array",
			expr: &model.ArrayLit{Items: []model.ArrayItem{{Key: lit("k"), Val: v("x")}}},
			want: `__array(__pair("k", v_x))`,
			vars: []string{"x"},
		},
		{
			name: "empty array",
			expr: &model.ArrayLit{},
			want: `__array()`,
		},
		{
			name: "concat",
			expr: &model.Binary{Op: ".", Left: v("a"), Right: lit(" b")},
			want: `__concat(v_a, " b")`,
			vars: []string{"a"},
		},
		{
			name: "arithmetic",
			expr: &model.Binary{Op: "+", Left: v("a"), Right: &model.Binary{Op: "%", Left: v("b"), Right: lit(2)}},
			want: `__arith("+", v_a, __arith("%", v_b, 2))`,
			vars: []string{"a", "b"},
		},
		{
			name: "identity comparison",
			expr: &model.Binary{Op: "===", Left: v("a"), Right: lit(1)},
			want: `(v_a) == (1)`,
			vars: []string{"a"},
		},
		{
			name: "not identical",
			expr: &model.Binary{Op: "!==", Left: v("a"), Right: lit(1)},
			want: `(v_a) != (1)`,
			vars: []string{"a"},
		},
		{
			name: "loose comparison",
			expr: &model.Binary{Op: ">=", Left: v("a"), Right: lit(1)},
			want: `(v_a) >= (1)`,
			vars: []string{"a"},
		},
		{
			name: "logical and",
			expr: &model.Binary{Op: "&&", Left: v("a"), Right: v("b")},
			want: `__bool(v_a) && __bool(v_b)`,
			vars: []string{"a", "b"},
		},
		{
			name: "logical or",
			expr: &model.Binary{Op: "||", Left: v("a"), Right: v("b")},
			want: `__bool(v_a) || __bool(v_b)`,
			vars: []string{"a", "b"},
		},
		{
			name: "negation and grouping",
			expr: &model.Unary{Op: "!", X: &model.Parenthesized{X: &model.Unary{Op: "-", X: v("a")}}},
			want: `!__bool((-(v_a)))`,
			vars: []string{"a"},
		},
		{
			name: "ternary",
			expr: &model.Ternary{Cond: v("a"), Then: lit(1), Else: lit(2)},
			want: `(__bool(v_a)) ? (1) : (2)`,
			vars: []string{"a"},
		},
		{
			name: "index",
			expr: &model.Index{Base: v("a"), Index: lit("k")},
			want: `__index(v_a, "k")`,
			vars: []string{"a"},
		},
		{
			name: "property",
			expr: &model.PropAccess{Base: v("obj"), Name: "field"},
			want: `__get(v_obj, "field")`,
			vars: []string{"obj"},
		},
		{
			name: "class constant",
			expr: &model.ClassConst{Class: "Foo", Name: "BAR"},
			want: `__classconst("Foo", "BAR")`,
		},
		{
			name: "cast",
			expr: &model.Cast{Type: "int", X: v("a")},
			want: `__cast("int", v_a)`,
			vars: []string{"a"},
		},
		{
			name: "assignment expression",
			expr: &model.AssignExpr{Target: v("a"), Op: "=", Value: lit(1)},
			want: `__set("a", 1)`,
			vars: nil,
		},
		{
			name: "call",
			expr: &model.Call{Name: "strlen", Args: []model.Expr{v("a")}},
			want: `strlen(v_a)`,
			vars: []string{"a"},
		},
		{
			name: "call without arguments",
			expr: &model.Call{Name: "time"},
			want: `time()`,
		},
		{
			name: "call by reference",
			expr: &model.Call{Name: "preg_match", Args: []model.Expr{lit("/a/"), v("s"), v("m")}},
			want: `preg_match("/a/", v_s, __ref("m"))`,
			vars: []string{"s", "m"},
		},
		{
			name: "namespaced call",
			expr: &model.Call{Name: `App\f`, Fallback: "f", Args: []model.Expr{lit(1)}},
			want: `__func("App\\f", "f", 1)`,
		},
		{
			name: "method call",
			expr: &model.MethodCall{Base: v("obj"), Method: "run", Args: []model.Expr{lit(1), v("a")}},
			want: `__call(v_obj, "run", 1, v_a)`,
			vars: []string{"obj", "a"},
		},
		{
			name: "method call without arguments",
			expr: &model.MethodCall{Base: v("obj"), Method: "run"},
			want: `__call(v_obj, "run")`,
			vars: []string{"obj"},
		},
		{
			name: "new",
			expr: &model.New{Class: "Foo", Args: []model.Expr{lit(1)}},
			want: `__new("Foo", 1)`,
		},
		{
			name: "new without arguments",
			expr: &model.New{Class: "Foo"},
			want: `__new("Foo")`,
		},
		{
			name: "this",
			expr: &model.PropAccess{Base: v("this"), Name: "id"},
			want: `__get(this, "id")`,
			vars: []string{"this"},
		},
		{
			name: "repeated variable is collected once",
			expr: &model.Binary{Op: ".", Left: v("a"), Right: v("a")},
			want: `__concat(v_a, v_a)`,
			vars: []string{"a"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTranspiler()
			src, vars, err := tr.Transpile(tc.expr)
			if err != nil {
				t.Fatalf("Transpile: %v", err)
			}
			if src != tc.want {
				t.Errorf("src\n got %s\nwant %s", src, tc.want)
			}
			if len(vars) != len(tc.vars) {
				t.Fatalf("vars = %v, want %v", vars, tc.vars)
			}
			for i, name := range tc.vars {
				if vars[i] != name {
					t.Fatalf("vars = %v, want %v", vars, tc.vars)
				}
				if got, want := tr.Idents()[i], varIdent(name); got != want {
					t.Errorf("ident %d = %q, want %q", i, got, want)
				}
			}
		})
	}
}

// TestTranspileMarkers covers the node kinds that register out-of-band state:
// closures and deferred evaluation markers.
func TestTranspileMarkers(t *testing.T) {
	tr := NewTranspiler()

	cl := &model.Closure{}
	src, _, err := tr.Transpile(&model.Call{Name: "usort", Args: []model.Expr{&model.Var{Name: "a"}, cl}})
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	if want := `usort(v_a, __cl0)`; src != want {
		t.Fatalf("src = %s, want %s", src, want)
	}
	if got := tr.Closures()["__cl0"]; got != cl {
		t.Fatalf("closure __cl0 = %v, want the closure node", got)
	}
	if len(tr.Exprs()) != 0 {
		t.Fatalf("exprs = %v, want empty", tr.Exprs())
	}

	inc := &model.Include{Path: &model.Lit{Value: "x.php"}, Keyword: "include"}
	src, _, err = tr.Transpile(inc)
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	if want := `__eval("0__expr")`; src != want {
		t.Fatalf("src = %s, want %s", src, want)
	}
	if got := tr.Exprs()["0__expr"]; got != model.Expr(inc) {
		t.Fatalf("expr marker = %v, want the include node", got)
	}
	// Transpile resets: the closure from the previous run must be gone.
	if len(tr.Closures()) != 0 {
		t.Fatalf("closures = %v, want empty after reset", tr.Closures())
	}

	src, _, err = tr.Transpile(&model.Unary{Op: "++", X: &model.Var{Name: "i"}, Postfix: true})
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	if want := `__eval("0__expr")`; src != want {
		t.Fatalf("src = %s, want %s", src, want)
	}
}

// TestTranspileCalls pins the function names collected for lazy environment
// population. Every name emitted as a bare env identifier must be listed:
// Runtime.Eval installs exactly these and nothing else, so a missed name is a
// call to nil at runtime. Names routed through the __func helper must not be,
// since that helper resolves against the function table directly.
func TestTranspileCalls(t *testing.T) {
	v := func(name string) model.Expr { return &model.Var{Name: name} }
	lit := func(val any) model.Expr { return &model.Lit{Value: val} }

	cases := []struct {
		name  string
		expr  model.Expr
		calls []string
	}{
		{
			name:  "plain call",
			expr:  &model.Call{Name: "strlen", Args: []model.Expr{v("a")}},
			calls: []string{"strlen"},
		},
		{
			name:  "no call",
			expr:  &model.Binary{Op: ".", Left: v("a"), Right: lit("b")},
			calls: nil,
		},
		{
			name: "nested calls in argument position",
			expr: &model.Call{Name: "implode", Args: []model.Expr{
				lit(","),
				&model.Call{Name: "array_map", Args: []model.Expr{
					&model.Call{Name: "trim", Args: []model.Expr{v("a")}},
				}},
			}},
			calls: []string{"trim", "array_map", "implode"},
		},
		{
			name: "call inside an array literal, a ternary and a cast",
			expr: &model.Cast{Type: "int", X: &model.Ternary{
				Cond: &model.Call{Name: "is_array", Args: []model.Expr{v("a")}},
				Then: &model.ArrayLit{Items: []model.ArrayItem{
					{Key: &model.Call{Name: "key_of"}, Val: &model.Call{Name: "count", Args: []model.Expr{v("a")}}},
				}},
				Else: &model.Index{Base: &model.Call{Name: "explode"}, Index: lit(0)},
			}},
			calls: []string{"is_array", "key_of", "count", "explode"},
		},
		{
			name: "call inside a method call and a constructor argument",
			expr: &model.MethodCall{Base: &model.New{Class: "Foo", Args: []model.Expr{
				&model.Call{Name: "getenv", Args: []model.Expr{lit("HOME")}},
			}}, Method: "run", Args: []model.Expr{&model.Call{Name: "time"}}},
			calls: []string{"getenv", "time"},
		},
		{
			name:  "repeated call is collected once",
			expr:  &model.Binary{Op: ".", Left: &model.Call{Name: "time"}, Right: &model.Call{Name: "time"}},
			calls: []string{"time"},
		},
		{
			name:  "by-reference argument does not hide the call",
			expr:  &model.Call{Name: "preg_match", Args: []model.Expr{lit("/a/"), v("s"), v("m")}},
			calls: []string{"preg_match"},
		},
		{
			name:  "namespaced call goes through __func",
			expr:  &model.Call{Name: `App\f`, Args: []model.Expr{lit(1)}},
			calls: nil,
		},
		{
			name:  "call with a global fallback goes through __func",
			expr:  &model.Call{Name: "f", Fallback: "f", Args: []model.Expr{lit(1)}},
			calls: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTranspiler()
			if _, _, err := tr.Transpile(tc.expr); err != nil {
				t.Fatalf("Transpile: %v", err)
			}
			got := tr.Calls()
			if len(got) != len(tc.calls) {
				t.Fatalf("calls = %v, want %v", got, tc.calls)
			}
			for i, name := range tc.calls {
				if got[i] != name {
					t.Fatalf("calls = %v, want %v", got, tc.calls)
				}
			}
		})
	}
}

// TestTranspilerPoolIsolation asserts that a pooled transpiler does not leak
// the previous expression's variables or calls into the next one.
func TestTranspilerPoolIsolation(t *testing.T) {
	tr := acquireTranspiler()
	if _, vars, err := tr.Transpile(&model.Call{Name: "strlen", Args: []model.Expr{&model.Var{Name: "a"}}}); err != nil || len(vars) != 1 {
		t.Fatalf("Transpile = %v, %v", vars, err)
	}
	if len(tr.Calls()) != 1 {
		t.Fatalf("calls = %v, want [strlen]", tr.Calls())
	}
	releaseTranspiler(tr)

	tr = acquireTranspiler()
	defer releaseTranspiler(tr)
	src, vars, err := tr.Transpile(&model.Var{Name: "b"})
	if err != nil {
		t.Fatalf("Transpile: %v", err)
	}
	if src != "v_b" || len(vars) != 1 || vars[0] != "b" {
		t.Fatalf("src = %q, vars = %v, want v_b [b]", src, vars)
	}
	if len(tr.Calls()) != 0 {
		t.Fatalf("calls = %v, want empty after reset", tr.Calls())
	}
}
